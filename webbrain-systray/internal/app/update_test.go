package app

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/esokullu/webbrain-systray/internal/paths"
	"github.com/esokullu/webbrain-systray/internal/settings"
	"github.com/esokullu/webbrain-systray/internal/update"
)

func newTestApp(t *testing.T, version string) *App {
	t.Helper()
	t.Setenv("WBC_HOME", t.TempDir())
	store, err := settings.Open()
	if err != nil {
		t.Fatal(err)
	}
	return New(version, store)
}

func TestDevelopmentBuildsDoNotUpdate(t *testing.T) {
	a := newTestApp(t, "0.1.0-dev")
	if st := a.UpdateStatus(); !st.Dev || st.Supported || !st.Auto {
		t.Fatalf("status = %+v", st)
	}
}

func TestAutomaticInstallWaitsUntilIdle(t *testing.T) {
	a := newTestApp(t, "0.1.2")
	a.upd.release = &update.Release{Version: "0.1.3"}
	a.upd.pkg = "/downloads/0.1.3/package"
	idle := func(ago time.Duration) { a.lastActive.Store(time.Now().Add(-ago).UnixNano()) }

	idle(time.Minute)
	if a.autoInstallDue() {
		t.Fatal("installed one minute after the last request")
	}
	idle(11 * time.Minute)
	if !a.autoInstallDue() {
		t.Fatal("did not install after ten idle minutes")
	}
	if st := a.UpdateStatus(); st.State != "ready" || st.Latest != "0.1.3" {
		t.Fatalf("status = %+v", st)
	}

	a.activeReqs.Add(1)
	if a.autoInstallDue() {
		t.Fatal("installed during a request")
	}
	a.activeReqs.Add(-1)

	_, unsubscribe := a.Subscribe() // an open panel
	if a.autoInstallDue() {
		t.Fatal("installed while the panel is open")
	}
	unsubscribe()

	a.mu.Lock()
	a.logins["s"] = &login{Status: "wait"}
	a.mu.Unlock()
	if a.autoInstallDue() {
		t.Fatal("installed during a sign-in")
	}
	a.mu.Lock()
	delete(a.logins, "s")
	a.mu.Unlock()

	if _, err := a.Store.Update(func(s *settings.Settings) { s.ManualUpdates = true }); err != nil {
		t.Fatal(err)
	}
	if a.autoInstallDue() || a.UpdateStatus().Auto {
		t.Fatal("installed with automatic updates off")
	}
	if _, err := a.Store.Update(func(s *settings.Settings) { s.ManualUpdates = false }); err != nil {
		t.Fatal(err)
	}
	a.Headless = true
	if a.autoInstallDue() {
		t.Fatal("a headless companion installed by itself")
	}
}

// Only work a client asked for counts as use: not the companion's own calls
// to the proxy's management API (usage refreshes run every few minutes), not
// reads, not refused requests. A model session over a WebSocket counts for as
// long as it lasts.
func TestActivityCountsOnlyClientWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newTestApp(t, "0.1.2")
	e := gin.New()
	e.Use(a.activity)
	var during int64
	ok := func(c *gin.Context) { during = a.activeReqs.Load(); c.Status(http.StatusOK) }
	e.GET("/v1/models", ok)
	e.GET("/v1/responses", ok)
	e.POST("/v1/chat/completions", ok)
	e.POST("/v8/management/requests/api-call", ok)
	e.POST("/v1/messages", func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) })
	e.POST("/companion/api/update/check", ok)

	past := time.Now().Add(-time.Hour).UnixNano()
	serve := func(method, path string, websocket bool) (counted bool, inFlight int64) {
		a.lastActive.Store(past)
		during = 0
		r := httptest.NewRequest(method, path, nil)
		if websocket {
			r.Header.Set("Connection", "Upgrade")
			r.Header.Set("Upgrade", "websocket")
		}
		e.ServeHTTP(httptest.NewRecorder(), r)
		if a.activeReqs.Load() != 0 {
			t.Fatalf("%s %s left %d requests in flight", method, path, a.activeReqs.Load())
		}
		return a.lastActive.Load() != past, during
	}
	for _, tc := range []struct {
		method, path string
		websocket    bool
		counts       bool
	}{
		{http.MethodGet, "/v1/models", false, false},
		{http.MethodPost, "/v8/management/requests/api-call", false, false},
		{http.MethodPost, "/v1/messages", false, false}, // refused
		{http.MethodPost, "/v1/chat/completions", false, true},
		{http.MethodGet, "/v1/responses", true, true}, // a Codex WebSocket session
		{http.MethodPost, "/companion/api/update/check", false, true},
	} {
		counted, inFlight := serve(tc.method, tc.path, tc.websocket)
		if counted != tc.counts {
			t.Errorf("%s %s (websocket %v): counted %v, want %v", tc.method, tc.path, tc.websocket, counted, tc.counts)
		}
		if tc.counts && tc.path != "/v1/messages" && inFlight != 1 {
			t.Errorf("%s %s was not in flight while it ran", tc.method, tc.path)
		}
	}
}

// releaseFeed serves one Linux release whose program is body.
func releaseFeed(t *testing.T, version string, files map[string]string) {
	t.Helper()
	name, _ := update.PackageName(version, "linux", runtime.GOARCH)
	pkg := tarGz(t, files)
	sum := sha256.Sum256(pkg)
	sums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name)
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"tag_name": "systray-v" + version, "html_url": base + "/page",
				"assets": []map[string]any{
					{"name": name, "browser_download_url": base + "/files/pkg", "size": len(pkg)},
					{"name": "WebBrain-Companion-" + version + "-SHA256SUMS.txt", "browser_download_url": base + "/files/sums"},
				},
			}})
		case "/files/pkg":
			_, _ = w.Write(pkg)
		case "/files/sums":
			_, _ = w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	base = server.URL
	t.Setenv("WBC_UPDATE_FEED", server.URL+"/releases")
}

func installedProgram(t *testing.T, a *App) {
	t.Helper()
	a.exe = filepath.Join(t.TempDir(), "webbrain-companion")
	if err := os.WriteFile(a.exe, []byte("old program"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The whole update on Linux: find the release, download and check it,
// replace the program, quit, and start the new version in this process. When
// the new version does not start, the previous one is put back and started.
func TestInstallUpdateReplacesTheProgramAndRestarts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("installs the Linux package")
	}
	a := newTestApp(t, "0.1.2")
	installedProgram(t, a)
	releaseFeed(t, "0.1.3", map[string]string{"webbrain-companion/webbrain-companion": "new program", "webbrain-companion/README.md": "readme"})

	quit := 0
	a.Quit = func() { quit++ }
	a.installUpdate()
	if quit != 1 || !a.RestartPending() {
		t.Fatalf("quit %d times, restart pending %v; status %+v", quit, a.RestartPending(), a.UpdateStatus())
	}
	if got, _ := os.ReadFile(a.exe); string(got) != "new program" {
		t.Fatalf("the program was not replaced: %q", got)
	}

	var started []string
	a.exec = func(exe string, args ...string) error {
		got, _ := os.ReadFile(exe)
		started = append(started, string(got)+" "+strings.Join(args, " "))
		return errors.New("exec format error") // the new program does not start
	}
	a.Restart()
	want := []string{"new program --background --after-update", "old program --background --after-update"}
	if strings.Join(started, "|") != strings.Join(want, "|") {
		t.Fatalf("started %q, want the new version and then the previous one %q", started, want)
	}
	if got, _ := os.ReadFile(a.exe); string(got) != "old program" {
		t.Fatalf("the previous program was not put back: %q", got)
	}

	// Started again, the previous version knows the update did not take and
	// does not try it again by itself.
	b := New("0.1.2", a.Store)
	b.exe = a.exe
	b.readAttempt()
	b.upd.release = &update.Release{Version: "0.1.3"}
	b.upd.pkg = "/downloads/package"
	b.lastActive.Store(time.Now().Add(-time.Hour).UnixNano())
	if b.autoInstallDue() {
		t.Fatal("retried a failed version by itself")
	}
	if st := b.UpdateStatus(); st.State != "error" || !strings.Contains(st.Error, "0.1.3") {
		t.Fatalf("status = %+v, want the failed update reported", st)
	}
	// A newer release is tried again.
	b.upd.release = &update.Release{Version: "0.1.4"}
	if !b.autoInstallDue() {
		t.Fatal("a newer release was not installed")
	}
}

// A failed install is reported and not retried every half minute.
func TestFailedInstallIsReportedNotRetried(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("installs the Linux package")
	}
	a := newTestApp(t, "0.1.2")
	installedProgram(t, a)
	releaseFeed(t, "0.1.3", map[string]string{"webbrain-companion/README.md": "a package without the program"})
	a.Quit = func() { t.Fatal("quit after a failed install") }
	a.installUpdate()
	st := a.UpdateStatus()
	if st.State != "error" || !strings.Contains(st.Error, "incomplete") {
		t.Fatalf("status = %+v", st)
	}
	a.lastActive.Store(time.Now().Add(-time.Hour).UnixNano())
	if a.autoInstallDue() {
		t.Fatal("a failed version is due again")
	}
	if a.RestartPending() {
		t.Fatal("restart pending after a failed install")
	}
	if got, _ := os.ReadFile(a.exe); string(got) != "old program" {
		t.Fatalf("a failed install changed the program: %q", got)
	}
}

func TestAttemptRecordClearsAfterASuccessfulUpdate(t *testing.T) {
	a := newTestApp(t, "0.1.2")
	a.writeAttempt("0.1.3")
	if _, err := os.Stat(filepath.Join(paths.Updates(), "attempt.json")); err != nil {
		t.Fatal(err)
	}
	b := New("0.1.3", a.Store) // the new version started
	b.readAttempt()
	if _, err := os.Stat(filepath.Join(paths.Updates(), "attempt.json")); !os.IsNotExist(err) {
		t.Fatal("the record of a finished update was kept")
	}
	if st := b.UpdateStatus(); st.State != "idle" || st.Error != "" {
		t.Fatalf("status = %+v", st)
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pkg.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
