package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		newer bool
	}{
		{"0.1.3", "0.1.2", true},
		{"0.2.0", "0.1.9", true},
		{"1.0.0", "0.99.99", true},
		{"0.1.10", "0.1.9", true},
		{"0.1.2", "0.1.2", false},
		{"0.1.1", "0.1.2", false},
		// Development and CI builds never update, and never count as releases.
		{"0.1.3", "0.1.0-dev", false},
		{"0.1.3", "0.0.0-abc1234", false},
		{"0.1.3-rc1", "0.1.2", false},
		{"0.1", "0.0.9", false},
	} {
		if got := Newer(tc.a, tc.b); got != tc.newer {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.newer)
		}
	}
}

func TestPackageNames(t *testing.T) {
	for _, tc := range []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "WebBrain-Companion-0.1.3-macos-universal.zip"},
		{"darwin", "amd64", "WebBrain-Companion-0.1.3-macos-universal.zip"},
		{"windows", "amd64", "WebBrain-Companion-0.1.3-windows-setup.exe"},
		{"windows", "arm64", "WebBrain-Companion-0.1.3-windows-setup.exe"},
		{"linux", "amd64", "WebBrain-Companion-0.1.3-linux-x64.tar.gz"},
		{"linux", "arm64", "WebBrain-Companion-0.1.3-linux-arm64.tar.gz"},
	} {
		if got, ok := PackageName("0.1.3", tc.goos, tc.goarch); !ok || got != tc.want {
			t.Errorf("PackageName(%s/%s) = %q, %v; want %q", tc.goos, tc.goarch, got, ok, tc.want)
		}
	}
	if _, ok := PackageName("0.1.3", "linux", "386"); ok {
		t.Error("linux/386 has no package")
	}
}

func TestParseSums(t *testing.T) {
	a := strings.Repeat("ab", 32)
	b := strings.Repeat("CD", 32)
	sums := ParseSums([]byte(a + "  WebBrain-Companion-0.1.3-linux-x64.tar.gz\n" + b + " *WebBrain-Companion-0.1.3-windows-setup.exe\nnot a line\n"))
	if sums["WebBrain-Companion-0.1.3-linux-x64.tar.gz"] != a || sums["WebBrain-Companion-0.1.3-windows-setup.exe"] != strings.ToLower(b) || len(sums) != 2 {
		t.Fatalf("ParseSums = %v", sums)
	}
}

func TestFeedOverrideStaysOnThisComputer(t *testing.T) {
	t.Setenv("WBC_UPDATE_FEED", "https://example.com/releases")
	if got, test := feed(); got != defaultFeed || test {
		t.Fatalf("feed() = %q, %v; a non-loopback override must be ignored", got, test)
	}
	t.Setenv("WBC_UPDATE_FEED", "http://127.0.0.1:9/releases")
	if got, test := feed(); got != "http://127.0.0.1:9/releases" || !test {
		t.Fatalf("feed() = %q, %v", got, test)
	}
	if !allowedDownload("https://github.com/webbrain-one/webbrain/releases/download/systray-v0.1.3/x.zip", false) {
		t.Error("the repository's release download path must be accepted")
	}
	for _, u := range []string{
		"http://github.com/webbrain-one/webbrain/releases/download/systray-v0.1.3/x.zip",
		"https://github.com/someone/else/releases/download/systray-v0.1.3/x.zip",
		"https://example.com/webbrain-one/webbrain/releases/download/x.zip",
		"http://127.0.0.1/x.zip",
	} {
		if allowedDownload(u, false) {
			t.Errorf("allowedDownload(%q) = true", u)
		}
	}
}

// releaseServer serves a GitHub-like release list and the files it names.
type releaseServer struct {
	*httptest.Server
	files map[string][]byte
}

func newReleaseServer(t *testing.T, build func(base string) []ghRelease) *releaseServer {
	t.Helper()
	rs := &releaseServer{files: map[string][]byte{}}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases" {
			if r.Header.Get("User-Agent") == "" {
				http.Error(w, "GitHub requires a User-Agent", http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(build(rs.URL))
			return
		}
		data, ok := rs.files[strings.TrimPrefix(r.URL.Path, "/files/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(rs.Close)
	t.Setenv("WBC_UPDATE_FEED", rs.URL+"/releases")
	return rs
}

func assetsFor(base string, names ...string) []Asset {
	var out []Asset
	for _, n := range names {
		out = append(out, Asset{Name: n, URL: base + "/files/" + n})
	}
	return out
}

func TestLatestPicksNewestPublishedCompanionRelease(t *testing.T) {
	pkg := func(v string) string { n, _ := PackageName(v, "linux", "amd64"); return n }
	newReleaseServer(t, func(base string) []ghRelease {
		return []ghRelease{
			{Tag: "v45.0.0", Page: base + "/ext", Assets: assetsFor(base, pkg("45.0.0"), sumsName("45.0.0"))},
			{Tag: "systray-v0.1.5", Prerelease: true, Assets: assetsFor(base, pkg("0.1.5"), sumsName("0.1.5"))},
			{Tag: "systray-v0.1.4", Draft: true, Assets: assetsFor(base, pkg("0.1.4"), sumsName("0.1.4"))},
			{Tag: "systray-v0.2.0", Assets: assetsFor(base, pkg("0.2.0"))}, // no SHA256SUMS
			{Tag: "systray-v0.1.3", Page: base + "/0.1.3", Assets: assetsFor(base, pkg("0.1.3"), sumsName("0.1.3"), "WebBrain-Companion-0.1.3-macos-universal.zip")},
			{Tag: "systray-v0.1.2", Assets: assetsFor(base, pkg("0.1.2"), sumsName("0.1.2"))},
		}
	})
	rel, err := latest(context.Background(), "0.1.2", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "0.1.3" || rel.Package.Name != pkg("0.1.3") || rel.Sums.Name != sumsName("0.1.3") {
		t.Fatalf("latest = %+v, want 0.1.3", rel)
	}
	if rel, err := latest(context.Background(), "0.1.3", "linux", "amd64"); err != nil || rel != nil {
		t.Fatalf("on the newest version: %+v, %v", rel, err)
	}
	if rel, err := latest(context.Background(), "0.1.0-dev", "linux", "amd64"); err != nil || rel != nil {
		t.Fatalf("a development build must not update: %+v, %v", rel, err)
	}
	// A release without this system's package is skipped.
	if rel, err := latest(context.Background(), "0.1.2", "linux", "arm64"); err != nil || rel != nil {
		t.Fatalf("linux/arm64 has no package here: %+v, %v", rel, err)
	}
}

func TestLatestReportsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()
	t.Setenv("WBC_UPDATE_FEED", server.URL)
	if _, err := latest(context.Background(), "0.1.2", "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want the HTTP status", err)
	}
}

func TestDownloadChecksThePackage(t *testing.T) {
	body := []byte("the new companion")
	sum := sha256.Sum256(body)
	name, _ := PackageName("0.1.3", "linux", "amd64")
	rs := newReleaseServer(t, func(string) []ghRelease { return nil })
	rs.files[name] = body
	rs.files[sumsName("0.1.3")] = []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name))
	rel := &Release{
		Version: "0.1.3",
		Package: Asset{Name: name, URL: rs.URL + "/files/" + name, Size: int64(len(body))},
		Sums:    Asset{Name: sumsName("0.1.3"), URL: rs.URL + "/files/" + sumsName("0.1.3")},
	}
	dir := t.TempDir()
	path, err := Download(context.Background(), rel, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(body) {
		t.Fatalf("downloaded %q", got)
	}

	// A package that does not match its checksum is refused and removed.
	rs.files[name] = []byte("tampered companion")
	rel.Package.Size = int64(len(rs.files[name]))
	other := t.TempDir()
	if _, err := Download(context.Background(), rel, other, nil); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v, want a checksum mismatch", err)
	}
	if entries, _ := os.ReadDir(other); len(entries) != 0 {
		t.Fatalf("a refused package was left behind: %v", entries)
	}

	// A package missing from SHA256SUMS is refused.
	rs.files[sumsName("0.1.3")] = []byte(fmt.Sprintf("%s  something-else\n", hex.EncodeToString(sum[:])))
	if _, err := Download(context.Background(), rel, t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("err = %v, want not listed", err)
	}
}

// Extension releases share the list: the Companion release may sit on a later
// page, and reading stops once the running version's release has passed.
func TestLatestReadsLaterPages(t *testing.T) {
	pkg := func(v string) string { n, _ := PackageName(v, "linux", "amd64"); return n }
	var base string
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		var list []ghRelease
		switch page {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/releases?page=2>; rel="next", <%s/releases?page=3>; rel="last"`, base, base))
			for i := 0; i < 100; i++ {
				list = append(list, ghRelease{Tag: fmt.Sprintf("v46.0.%d", i)})
			}
		case "2":
			w.Header().Set("Link", fmt.Sprintf(`<%s/releases?page=3>; rel="next"`, base))
			list = []ghRelease{
				{Tag: "systray-v0.1.3", Assets: assetsFor(base, pkg("0.1.3"), sumsName("0.1.3"))},
				{Tag: "v45.0.0"},
				{Tag: "systray-v0.1.2", Assets: assetsFor(base, pkg("0.1.2"), sumsName("0.1.2"))},
			}
		default:
			t.Errorf("read page %q after passing the running version", page)
		}
		_ = json.NewEncoder(w).Encode(list)
	}))
	defer server.Close()
	base = server.URL
	t.Setenv("WBC_UPDATE_FEED", server.URL+"/releases")
	rel, err := latest(context.Background(), "0.1.2", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "0.1.3" {
		t.Fatalf("latest = %+v, want 0.1.3 from the second page", rel)
	}
	if len(pages) != 2 {
		t.Fatalf("read pages %q", pages)
	}
}

func TestNextPageStaysOnTheFeedHost(t *testing.T) {
	feed := "https://api.github.com/repos/webbrain-one/webbrain/releases?per_page=100"
	next := "https://api.github.com/repositories/1/releases?per_page=100&page=2"
	if got := nextPage(`<`+next+`>; rel="next", <https://api.github.com/repositories/1/releases?page=9>; rel="last"`, feed); got != next {
		t.Fatalf("nextPage = %q", got)
	}
	for _, link := range []string{
		`<https://example.com/releases?page=2>; rel="next"`,
		`<http://api.github.com/releases?page=2>; rel="next"`,
		`<https://api.github.com/releases?page=9>; rel="last"`,
		``,
	} {
		if got := nextPage(link, feed); got != "" {
			t.Errorf("nextPage(%q) = %q", link, got)
		}
	}
}
