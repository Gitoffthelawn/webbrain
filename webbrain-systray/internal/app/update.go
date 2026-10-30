package app

// Updates. Shortly after it starts, and every six hours, the companion asks
// GitHub for a newer release (internal/update). With automatic updates on, the
// default, it downloads the release, checks it against the release's
// SHA256SUMS, and installs it once nothing has used the companion for ten
// minutes. On macOS and Linux the new version then replaces this process when
// it quits (Restart); on Windows the setup program starts it. The tray menu
// and the panel can install a downloaded update at once.
//
// An automatic install is tried once per version: when it fails, here or after
// the restart (the attempt record), the companion reports the error and waits
// for a newer release or for Update now.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/esokullu/webbrain-systray/internal/d1"
	"github.com/esokullu/webbrain-systray/internal/download"
	"github.com/esokullu/webbrain-systray/internal/logx"
	"github.com/esokullu/webbrain-systray/internal/paths"
	"github.com/esokullu/webbrain-systray/internal/update"
)

const (
	updateFirstCheck = 2 * time.Minute
	updateEvery      = 6 * time.Hour
	updateIdle       = 10 * time.Minute
	updateTick       = 30 * time.Second
	// A tick this late means the computer slept; waking up counts as use.
	updateWakeGap = 3 * updateTick
	// A download that makes no progress this long is abandoned until the
	// next check.
	updateStall       = 2 * time.Minute
	updateMaxDownload = 2 * time.Hour
)

var restartArgs = []string{"--background", "--after-update"}

const msgHeadless = "With --headless the companion only reports new versions. Run webbrain-companion --update, then restart it."

// UpdateStatus is what the panel shows about updates.
type UpdateStatus struct {
	// Auto installs a downloaded update once the companion is idle.
	Auto bool `json:"auto"`
	// Supported: this copy can replace itself. Reason says why it cannot.
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
	// Dev: a development build, which never updates.
	Dev bool `json:"dev,omitempty"`
	// State is idle, checking, downloading, ready, installing or error.
	State     string             `json:"state"`
	Current   string             `json:"current"`
	Latest    string             `json:"latest,omitempty"`
	Page      string             `json:"page,omitempty"`
	CheckedAt *time.Time         `json:"checked_at,omitempty"`
	Progress  *download.Progress `json:"progress,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type updater struct {
	mu        sync.Mutex
	busy      string // "", checking, downloading or installing
	release   *update.Release
	pkg       string // the release's package, downloaded and verified
	failed    string // a version whose install failed: not retried automatically
	checkedAt time.Time
	err       string
	reason    string // why this copy cannot replace itself
	progress  *download.Progress
	kick      chan struct{}
	// restart: an installed update replaces this process when it quits.
	restart *update.Target
}

var errNoUpdate = errors.New("no update is available")

func (u *updater) begin(state string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy != "" {
		return false
	}
	u.busy = state
	return true
}

func (u *updater) latest() *update.Release {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.release
}

func (u *updater) setReason(reason string) {
	u.mu.Lock()
	u.reason = reason
	u.mu.Unlock()
}

// UpdateStatus reports the update state for the panel and the tray.
func (a *App) UpdateStatus() UpdateStatus {
	st := UpdateStatus{Auto: !a.Store.Get().ManualUpdates, Current: a.Version, State: "idle"}
	if _, ok := update.ParseVersion(a.Version); !ok {
		st.Dev = true
		return st
	}
	u := &a.upd
	u.mu.Lock()
	defer u.mu.Unlock()
	st.Supported, st.Reason, st.Error = u.reason == "", u.reason, u.err
	if !u.checkedAt.IsZero() {
		at := u.checkedAt
		st.CheckedAt = &at
	}
	if u.release != nil {
		st.Latest, st.Page = u.release.Version, u.release.Page
	}
	switch {
	case u.busy != "":
		st.State = u.busy
	case u.err != "":
		st.State = "error"
	case u.pkg != "":
		st.State = "ready"
	}
	if u.busy == "downloading" && u.progress != nil {
		p := *u.progress
		st.Progress = &p
	}
	return st
}

// CheckForUpdate asks the update loop to check now (and, with automatic
// updates on, to download what it finds).
func (a *App) CheckForUpdate() {
	select {
	case a.upd.kick <- struct{}{}:
	default:
	}
}

// InstallUpdate installs the newest release now, downloading it first when
// needed, and restarts the companion. It returns at once; the status reports
// progress and failures.
func (a *App) InstallUpdate() { go a.installUpdate() }

func (a *App) updateLoop() {
	update.Cleanup(a.exe, paths.Updates(), a.Version)
	if _, ok := update.ParseVersion(a.Version); !ok {
		return
	}
	a.readAttempt()
	if a.Headless {
		a.upd.setReason(msgHeadless)
	} else if _, err := update.Locate(a.exe); err != nil {
		a.upd.setReason(err.Error())
	}
	next := time.Now().Add(updateFirstCheck).Round(0)
	last := time.Now().Round(0)
	tick := time.NewTicker(updateTick)
	defer tick.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.upd.kick:
			next = time.Now().Round(0)
		case <-tick.C:
		}
		// Wall-clock time: the monotonic clock stops while a Mac sleeps. A
		// late tick means the computer slept; someone may be about to use
		// the companion again.
		now := time.Now().Round(0)
		if now.Sub(last) > updateWakeGap {
			a.lastActive.Store(now.UnixNano())
		}
		last = now
		if !now.Before(next) {
			next = now.Add(updateEvery)
			a.checkUpdate()
			if a.autoDownload() {
				_, _ = a.downloadUpdate()
			}
		}
		if a.autoInstallDue() {
			a.installUpdate()
		}
	}
}

func (a *App) autoDownload() bool {
	if a.Headless || a.Store.Get().ManualUpdates {
		return false
	}
	a.upd.mu.Lock()
	defer a.upd.mu.Unlock()
	return a.upd.release != nil && a.upd.pkg == "" && a.upd.reason == "" && a.upd.failed != a.upd.release.Version
}

func (a *App) checkUpdate() {
	if !a.upd.begin("checking") {
		return
	}
	a.changed()
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	rel, err := update.Latest(ctx, a.Version)
	cancel()
	u := &a.upd
	u.mu.Lock()
	u.busy = ""
	u.checkedAt = time.Now()
	switch {
	case err != nil:
		u.err = err.Error()
	default:
		if rel == nil || u.release == nil || rel.Version != u.release.Version {
			u.pkg = ""
		}
		u.release = rel
		// A failed version keeps its message until a newer one comes out.
		if rel == nil || rel.Version != u.failed {
			u.err = ""
		}
	}
	u.mu.Unlock()
	a.changed()
	switch {
	case err != nil:
		logx.Printf("update check: %v", err)
	case rel != nil:
		logx.Printf("update: WebBrain Companion %s is available", rel.Version)
	}
}

// downloadUpdate fetches and verifies the newest release's package, once.
func (a *App) downloadUpdate() (string, error) {
	u := &a.upd
	u.mu.Lock()
	rel, pkg := u.release, u.pkg
	switch {
	case rel == nil:
		u.mu.Unlock()
		return "", errNoUpdate
	case pkg != "":
		u.mu.Unlock()
		return pkg, nil
	case u.busy != "":
		u.mu.Unlock()
		return "", errors.New("an update step is already running")
	}
	u.busy, u.progress = "downloading", nil
	u.mu.Unlock()
	a.changed()

	ctx, cancel := context.WithTimeout(a.ctx, updateMaxDownload)
	defer cancel()
	var lastProgress atomic.Int64
	lastProgress.Store(time.Now().UnixNano())
	stalled := make(chan struct{})
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if time.Since(time.Unix(0, lastProgress.Load())) > updateStall {
					close(stalled)
					cancel()
					return
				}
			}
		}
	}()
	var shown time.Time
	path, err := update.Download(ctx, rel, filepath.Join(paths.Updates(), rel.Version), func(p download.Progress) {
		lastProgress.Store(time.Now().UnixNano())
		u.mu.Lock()
		u.progress = &p
		u.mu.Unlock()
		if time.Since(shown) > time.Second {
			shown = time.Now()
			a.changed()
		}
	})
	select {
	case <-stalled:
		err = errors.New("the download stopped making progress; it resumes with the next check")
	default:
	}
	u.mu.Lock()
	u.busy, u.progress = "", nil
	if err != nil {
		u.err = fmt.Sprintf("download %s: %v", rel.Version, err)
	} else if u.release != nil && u.release.Version == rel.Version {
		u.pkg = path
		if u.failed != rel.Version {
			u.err = ""
		}
	}
	u.mu.Unlock()
	a.changed()
	if err != nil {
		logx.Printf("update download: %v", err)
		return "", err
	}
	logx.Printf("update: %s downloaded and verified", rel.Version)
	return path, nil
}

func (a *App) installUpdate() {
	if a.Headless {
		return
	}
	if a.upd.latest() == nil {
		a.checkUpdate()
	}
	rel := a.upd.latest()
	if rel == nil {
		return
	}
	target, err := update.Locate(a.exe)
	if err != nil {
		a.upd.setReason(err.Error())
		a.changed()
		return
	}
	a.upd.setReason("")
	pkg, err := a.downloadUpdate()
	if err != nil {
		return
	}
	if !a.upd.begin("installing") {
		return
	}
	a.changed()
	logx.Printf("update: installing %s", rel.Version)
	a.writeAttempt(rel.Version)
	if target.Kind == "installer" {
		// The setup program waits for this process to exit, installs, and
		// starts the new version.
		err = update.StartInstaller(pkg, os.Getpid())
	} else if err = update.Install(pkg, target); err == nil {
		a.upd.mu.Lock()
		a.upd.restart = &target
		a.upd.mu.Unlock()
	}
	if err != nil {
		logx.Printf("update install: %v", err)
		a.installFailed(rel.Version, fmt.Sprintf("install %s: %v", rel.Version, err))
		return
	}
	logx.Printf("update: %s installed; restarting", rel.Version)
	if a.Quit != nil {
		a.Quit()
	}
}

// installFailed reports a failed install and stops automatic retries of that
// version.
func (a *App) installFailed(version, msg string) {
	u := &a.upd
	u.mu.Lock()
	u.busy, u.err, u.failed = "", msg, version
	u.mu.Unlock()
	a.changed()
}

// RestartPending reports whether an installed update should replace this
// process once it has shut down (macOS and Linux).
func (a *App) RestartPending() bool {
	a.upd.mu.Lock()
	defer a.upd.mu.Unlock()
	return a.upd.restart != nil
}

// Restart replaces this process with the installed update; main calls it
// after shutdown, with the single-instance lock released. When the new
// version cannot start, it puts the previous version back and starts that.
// It returns only when neither starts.
func (a *App) Restart() {
	a.upd.mu.Lock()
	target := a.upd.restart
	a.upd.mu.Unlock()
	if target == nil {
		return
	}
	err := a.exec(a.exe, restartArgs...)
	logx.Printf("update: start the new version: %v", err)
	if err := update.Rollback(*target); err != nil {
		logx.Printf("update: put the previous version back: %v", err)
		return
	}
	err = a.exec(a.exe, restartArgs...)
	logx.Printf("update: start the previous version: %v", err)
}

// attempt records an install about to start. After the restart, a version
// still older than the attempt's means the install did not take.
type attempt struct {
	Version string    `json:"version"`
	From    string    `json:"from"`
	At      time.Time `json:"at"`
}

func attemptPath() string { return filepath.Join(paths.Updates(), "attempt.json") }

func (a *App) writeAttempt(version string) {
	data, _ := json.Marshal(attempt{Version: version, From: a.Version, At: time.Now()})
	if err := os.MkdirAll(paths.Updates(), 0o700); err == nil {
		err = os.WriteFile(attemptPath(), data, 0o600)
		if err != nil {
			logx.Printf("update: record attempt: %v", err)
		}
	}
}

func (a *App) readAttempt() {
	data, err := os.ReadFile(attemptPath())
	if err != nil {
		return
	}
	var at attempt
	if json.Unmarshal(data, &at) != nil || !update.Newer(at.Version, a.Version) {
		// The update took (or the record is unreadable).
		_ = os.Remove(attemptPath())
		return
	}
	logx.Printf("update: the update to %s did not finish; still %s", at.Version, a.Version)
	a.upd.mu.Lock()
	a.upd.failed = at.Version
	a.upd.err = fmt.Sprintf("The update to %s didn't finish. Choose Update now to try again, or download it by hand.", at.Version)
	a.upd.mu.Unlock()
}

// autoInstallDue: a verified update waits, its version has not failed
// before, and the companion has been idle long enough.
func (a *App) autoInstallDue() bool {
	if a.Headless || a.Store.Get().ManualUpdates {
		return false
	}
	a.upd.mu.Lock()
	ready := a.upd.busy == "" && a.upd.pkg != "" && a.upd.reason == "" &&
		a.upd.release != nil && a.upd.failed != a.upd.release.Version
	a.upd.mu.Unlock()
	return ready && a.idleFor() >= updateIdle
}

// idleFor is how long nothing has used the companion: no request that does
// work for a client, no open panel, no local or decision model being
// installed or started, no sign-in in progress.
func (a *App) idleFor() time.Duration {
	if a.activeReqs.Load() > 0 || a.listeners() > 0 || a.Local.Busy() || a.loginPending() {
		return 0
	}
	switch a.D1.Status().Phase {
	case d1.PhaseDownloading, d1.PhaseInstalling, d1.PhaseLoading:
		return 0
	}
	return time.Duration(time.Now().UnixNano() - a.lastActive.Load())
}

func (a *App) loginPending() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, l := range a.logins {
		if l.Status == "wait" {
			return true
		}
	}
	return false
}

// clientWork reports whether a request does work for a client: a model call
// or a model session (WebSocket), a decision, or a change made in the panel.
// Lists and status reads (GET) are not work, and neither is the companion's
// own traffic to the proxy's management API (usage refreshes, for one).
func clientWork(c *gin.Context) bool {
	p := c.Request.URL.Path
	if strings.HasPrefix(p, "/v8/management") || strings.HasPrefix(p, "/v0/management") {
		return false
	}
	if c.IsWebsocket() {
		return apiPath(p)
	}
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return apiPath(p) || strings.HasPrefix(p, "/companion/")
}

// activity records use of the companion for the automatic update. A request
// counts while it runs; when it ends it marks the companion as used, unless
// it was refused (a web page without the key, say).
func (a *App) activity(c *gin.Context) {
	if !clientWork(c) {
		c.Next()
		return
	}
	a.activeReqs.Add(1)
	defer a.activeReqs.Add(-1)
	c.Next()
	if s := c.Writer.Status(); s != http.StatusUnauthorized && s != http.StatusForbidden {
		a.lastActive.Store(time.Now().UnixNano())
	}
}
