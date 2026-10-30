// Package update keeps WebBrain Companion current. A new version is a
// published (not draft, not pre-release) GitHub release of
// webbrain-one/webbrain tagged systray-v<version>, as the WebBrain Companion
// workflow creates it. Every release carries a SHA256SUMS file, and a package
// is installed only when its checksum matches the one listed there.
package update

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/esokullu/webbrain-systray/internal/download"
)

const (
	// TagPrefix marks Companion releases among the repository's other releases.
	TagPrefix = "systray-v"
	// ReleasesPage lists every release, for downloading one by hand.
	ReleasesPage = "https://github.com/webbrain-one/webbrain/releases"

	defaultFeed  = "https://api.github.com/repos/webbrain-one/webbrain/releases?per_page=100"
	downloadHost = "github.com"
	downloadPath = "/webbrain-one/webbrain/releases/download/"
	userAgent    = "WebBrain-Companion-Updater/1 (+https://webbrain.one/companion)"
)

// Asset is one file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is a newer version and the files this computer needs from it.
type Release struct {
	Version string
	Page    string
	Package Asset // the package for this system
	Sums    Asset // the release's SHA256SUMS file
}

type ghRelease struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Page       string  `json:"html_url"`
	Assets     []Asset `json:"assets"`
}

var client = &http.Client{
	Timeout: 90 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// feed returns the address of the release list. WBC_UPDATE_FEED points tests
// at a server on this computer; any other address is ignored, so a changed
// environment cannot send the companion to another host.
func feed() (string, bool) {
	if v := os.Getenv("WBC_UPDATE_FEED"); v != "" {
		if u, err := url.Parse(v); err == nil && (u.Scheme == "http" || u.Scheme == "https") && loopback(u.Hostname()) {
			return v, true
		}
	}
	return defaultFeed, false
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// allowedDownload accepts release files from the repository's own download
// path (GitHub then redirects to its file storage), or from the local test
// server.
func allowedDownload(raw string, testFeed bool) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if testFeed {
		return (u.Scheme == "http" || u.Scheme == "https") && loopback(u.Hostname())
	}
	return u.Scheme == "https" && u.Host == downloadHost && strings.HasPrefix(u.Path, downloadPath)
}

// ParseVersion reads a release version, major.minor.patch. Builds with a
// suffix (a local "0.1.0-dev", CI's "0.0.0-abc1234") are not releases.
func ParseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		if p == "" || len(p) > 6 {
			return v, false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return v, false
			}
			n = n*10 + int(c-'0')
		}
		v[i] = n
	}
	return v, true
}

// Newer reports whether release version a comes after b. It is false when
// either is not a release version.
func Newer(a, b string) bool {
	va, okA := ParseVersion(a)
	vb, okB := ParseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// PackageName is the release file that installs version on goos/goarch, as
// scripts/build.sh names it.
func PackageName(version, goos, goarch string) (string, bool) {
	base := "WebBrain-Companion-" + version + "-"
	switch goos {
	case "windows":
		return base + "windows-setup.exe", true
	case "darwin":
		return base + "macos-universal.zip", true
	case "linux":
		switch goarch {
		case "amd64":
			return base + "linux-x64.tar.gz", true
		case "arm64":
			return base + "linux-arm64.tar.gz", true
		}
	}
	return "", false
}

func sumsName(version string) string { return "WebBrain-Companion-" + version + "-SHA256SUMS.txt" }

func findAsset(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Latest returns the newest published release after current that has a
// package for this system, or nil when current is the newest.
func Latest(ctx context.Context, current string) (*Release, error) {
	return latest(ctx, current, runtime.GOOS, runtime.GOARCH)
}

// maxPages bounds how far back the release list is read. The repository's
// other releases (the extension's) share the list, newest first.
const maxPages = 10

func latest(ctx context.Context, current, goos, goarch string) (*Release, error) {
	feedURL, testFeed := feed()
	var best *Release
	next := feedURL
	for page := 0; next != "" && page < maxPages; page++ {
		list, link, err := fetchReleases(ctx, next)
		if err != nil {
			return nil, err
		}
		passed := false
		for _, r := range list {
			if r.Draft || r.Prerelease || !strings.HasPrefix(r.Tag, TagPrefix) {
				continue
			}
			v := strings.TrimPrefix(r.Tag, TagPrefix)
			if _, ok := ParseVersion(v); ok && !Newer(v, current) {
				// Releases come newest first: from the running version on,
				// nothing further down is newer.
				passed = true
				continue
			}
			if !Newer(v, current) || (best != nil && !Newer(v, best.Version)) {
				continue
			}
			name, ok := PackageName(v, goos, goarch)
			if !ok {
				continue
			}
			pkg, okPkg := findAsset(r.Assets, name)
			sums, okSums := findAsset(r.Assets, sumsName(v))
			if !okPkg || !okSums || !allowedDownload(pkg.URL, testFeed) || !allowedDownload(sums.URL, testFeed) {
				continue
			}
			best = &Release{Version: v, Page: r.Page, Package: pkg, Sums: sums}
		}
		if passed {
			break
		}
		next = nextPage(link, feedURL)
	}
	return best, nil
}

func fetchReleases(ctx context.Context, pageURL string) ([]ghRelease, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("release list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("release list: %s", resp.Status)
	}
	var list []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&list); err != nil {
		return nil, "", fmt.Errorf("release list: %w", err)
	}
	return list, resp.Header.Get("Link"), nil
}

// nextPage returns the rel="next" address of a GitHub Link header when it
// stays on the feed's own host.
func nextPage(link, feedURL string) string {
	feed, err := url.Parse(feedURL)
	if err != nil {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(part), ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		u, err := url.Parse(strings.Trim(strings.TrimSpace(target), "<>"))
		if err != nil || u.Scheme != feed.Scheme || u.Host != feed.Host {
			return ""
		}
		return u.String()
	}
	return ""
}

// ParseSums reads sha256sum output: "<hex>  <name>" or "<hex> *<name>".
func ParseSums(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		sum, name, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || len(sum) != 64 {
			continue
		}
		if _, err := hex.DecodeString(sum); err != nil {
			continue
		}
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if name != "" {
			out[name] = strings.ToLower(sum)
		}
	}
	return out
}

func fetchSmall(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file is larger than expected")
	}
	return data, nil
}

// Download fetches rel's package into dir and checks it against the
// release's SHA256SUMS. A package already there and intact is reused. It
// returns the package's path.
func Download(ctx context.Context, rel *Release, dir string, onProgress func(download.Progress)) (string, error) {
	sums, err := fetchSmall(ctx, rel.Sums.URL, 64<<10)
	if err != nil {
		return "", fmt.Errorf("checksums: %w", err)
	}
	want := ParseSums(sums)[rel.Package.Name]
	if want == "" {
		return "", fmt.Errorf("%s is not listed in %s", rel.Package.Name, rel.Sums.Name)
	}
	dest := filepath.Join(dir, rel.Package.Name)
	if err := download.File(ctx, rel.Package.URL, dest, rel.Package.Size, want, onProgress); err != nil {
		return "", err
	}
	return dest, nil
}
