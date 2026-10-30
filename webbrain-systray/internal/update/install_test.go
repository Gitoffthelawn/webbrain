package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type entry struct {
	name string
	body string
	mode os.FileMode
}

func writeZip(t *testing.T, path string, entries ...entry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(e.body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func writeTarGz(t *testing.T, path string, entries ...entry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: int64(e.mode.Perm()), Size: int64(len(e.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(e.body))
	}
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
}

// fakeBundle lays out an installed app whose program prints "old".
func fakeBundle(t *testing.T, apps string) (bundle, exe string) {
	t.Helper()
	bundle = filepath.Join(apps, "WebBrain Companion.app")
	exe = filepath.Join(bundle, "Contents", "MacOS", "WebBrain Companion")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte("old plist"), 0o644); err != nil {
		t.Fatal(err)
	}
	return bundle, exe
}

func newMacPackage(t *testing.T, dir string, extra ...entry) string {
	pkg := filepath.Join(dir, "WebBrain-Companion-0.1.3-macos-universal.zip")
	entries := append([]entry{
		{"WebBrain Companion.app/Contents/MacOS/WebBrain Companion", "new", 0o755},
		{"WebBrain Companion.app/Contents/Info.plist", "new plist", 0o644},
		{"WebBrain Companion.app/Contents/Resources/app.icns", "icon", 0o644},
		{"README.md", "readme", 0o644},
		{"LICENSE", "license", 0o644},
	}, extra...)
	writeZip(t, pkg, entries...)
	return pkg
}

func TestLocate(t *testing.T) {
	apps := t.TempDir()
	bundle, exe := fakeBundle(t, apps)
	got, err := locate(exe, "darwin")
	if err != nil || got.Kind != "bundle" || got.Path != bundle {
		t.Fatalf("locate(darwin) = %+v, %v; want the bundle %s", got, err, bundle)
	}
	if _, err := locate("/private/var/folders/x/AppTranslocation/ABC/d/WebBrain Companion.app/Contents/MacOS/WebBrain Companion", "darwin"); !errors.Is(err, ErrTranslocated) {
		t.Fatalf("a translocated copy: err = %v", err)
	}
	if _, err := locate(filepath.Join(apps, "webbrain-companion"), "darwin"); err == nil {
		t.Fatal("a program outside a bundle cannot replace itself on macOS")
	}
	linuxExe := filepath.Join(t.TempDir(), "webbrain-companion")
	if got, err := locate(linuxExe, "linux"); err != nil || got.Kind != "binary" || got.Path != linuxExe {
		t.Fatalf("locate(linux) = %+v, %v", got, err)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		ro := t.TempDir()
		if err := os.Chmod(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(ro, 0o755)
		if _, err := locate(filepath.Join(ro, "webbrain-companion"), "linux"); err == nil {
			t.Fatal("a copy in a read-only folder cannot replace itself")
		}
	}
	if entries, _ := os.ReadDir(apps); len(entries) != 1 {
		t.Fatalf("the write check left files behind: %v", entries)
	}
}

func TestInstallBundleSwapsTheApp(t *testing.T) {
	apps := t.TempDir()
	bundle, exe := fakeBundle(t, apps)
	pkg := newMacPackage(t, t.TempDir())
	if err := Install(pkg, Target{Kind: "bundle", Path: bundle}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Fatalf("program after the update: %q", got)
	}
	if st, err := os.Stat(exe); err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("the new program is not executable: %v %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(bundle, "README.md")); !os.IsNotExist(err) {
		t.Fatal("files outside the app were copied into it")
	}
	old := filepath.Join(apps, ".WebBrain Companion.app.old")
	if got, _ := os.ReadFile(filepath.Join(old, "Contents", "MacOS", "WebBrain Companion")); string(got) != "old" {
		t.Fatalf("the replaced app was not kept for cleanup: %q", got)
	}
	entries, _ := os.ReadDir(apps)
	if len(entries) != 2 {
		t.Fatalf("Applications holds %v, want the app and its old copy", names(entries))
	}

	// The new version removes the old copy when it starts.
	downloads := t.TempDir()
	for _, v := range []string{"0.1.2", "0.1.3", "0.1.4", "junk"} {
		_ = os.MkdirAll(filepath.Join(downloads, v), 0o755)
	}
	_ = os.MkdirAll(filepath.Join(apps, ".wbc-update-123"), 0o755)
	Cleanup(exe, downloads, "0.1.3")
	if entries, _ := os.ReadDir(apps); len(entries) != 1 || entries[0].Name() != "WebBrain Companion.app" {
		t.Fatalf("after cleanup Applications holds %v", names(entries))
	}
	if entries, _ := os.ReadDir(downloads); len(entries) != 1 || entries[0].Name() != "0.1.4" {
		t.Fatalf("after cleanup downloads hold %v, want only 0.1.4", names(entries))
	}
}

func TestInstallBundleRefusesBrokenPackages(t *testing.T) {
	for name, build := range map[string]func(t *testing.T, dir string) string{
		"no program": func(t *testing.T, dir string) string {
			p := filepath.Join(dir, "pkg.zip")
			writeZip(t, p, entry{"WebBrain Companion.app/Contents/Info.plist", "plist", 0o644})
			return p
		},
		"program not executable": func(t *testing.T, dir string) string {
			p := filepath.Join(dir, "pkg.zip")
			writeZip(t, p, entry{"WebBrain Companion.app/Contents/MacOS/WebBrain Companion", "new", 0o644},
				entry{"WebBrain Companion.app/Contents/Info.plist", "plist", 0o644})
			return p
		},
		"a link": func(t *testing.T, dir string) string {
			return newMacPackage(t, dir, entry{"WebBrain Companion.app/Contents/Frameworks/x", "../../x", os.ModeSymlink | 0o777})
		},
		"not a zip": func(t *testing.T, dir string) string {
			p := filepath.Join(dir, "pkg.zip")
			_ = os.WriteFile(p, []byte("nope"), 0o644)
			return p
		},
	} {
		t.Run(name, func(t *testing.T) {
			apps := t.TempDir()
			bundle, exe := fakeBundle(t, apps)
			if err := Install(build(t, t.TempDir()), Target{Kind: "bundle", Path: bundle}); err == nil {
				t.Fatal("a broken package was installed")
			}
			if got, _ := os.ReadFile(exe); string(got) != "old" {
				t.Fatalf("the running app changed: %q", got)
			}
			if entries, _ := os.ReadDir(apps); len(entries) != 1 {
				t.Fatalf("Applications holds %v after a refused update", names(entries))
			}
		})
	}
}

func TestInstallBinaryReplacesTheProgram(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "webbrain-companion")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "WebBrain-Companion-0.1.3-linux-x64.tar.gz")
	writeTarGz(t, pkg,
		entry{"webbrain-companion/webbrain-companion", "new", 0o755},
		entry{"webbrain-companion/webbrain-companion.desktop", "desktop", 0o644},
		entry{"webbrain-companion/README.md", "readme", 0o644},
	)
	if err := Install(pkg, Target{Kind: "binary", Path: exe}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Fatalf("program after the update: %q", got)
	}
	if st, _ := os.Stat(exe); st.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", st.Mode())
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".webbrain-companion.old")); string(got) != "old" {
		t.Fatalf("the replaced program was not kept for a rollback: %q", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("the program's folder holds %v", names(entries))
	}

	broken := filepath.Join(t.TempDir(), "broken.tar.gz")
	writeTarGz(t, broken, entry{"webbrain-companion/README.md", "readme", 0o644})
	if err := Install(broken, Target{Kind: "binary", Path: exe}); err == nil {
		t.Fatal("a package without the program was installed")
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Fatalf("a refused update changed the program: %q", got)
	}
}

// Exec replaces the process: same process ID, the new program's arguments.
func TestExecReplacesTheProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS and Linux only")
	}
	if script := os.Getenv("WBC_EXEC_SCRIPT"); script != "" {
		err := Exec(script, "--background", "--after-update")
		t.Fatalf("Exec returned: %v", err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	script := filepath.Join(dir, "companion")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$$ $@\" > \""+marker+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestExecReplacesTheProcess$")
	cmd.Env = append(os.Environ(), "WBC_EXEC_SCRIPT="+script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%d --background --after-update", cmd.Process.Pid)
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("the new program saw %q, want %q", got, want)
	}
}

func TestRollbackPutsThePreviousVersionBack(t *testing.T) {
	apps := t.TempDir()
	bundle, exe := fakeBundle(t, apps)
	if err := Install(newMacPackage(t, t.TempDir()), Target{Kind: "bundle", Path: bundle}); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(Target{Kind: "bundle", Path: bundle}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Fatalf("app after the rollback: %q", got)
	}
	if entries, _ := os.ReadDir(apps); len(entries) != 1 {
		t.Fatalf("Applications holds %v after the rollback", names(entries))
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "webbrain-companion")
	_ = os.WriteFile(bin, []byte("old"), 0o755)
	pkg := filepath.Join(t.TempDir(), "pkg.tar.gz")
	writeTarGz(t, pkg, entry{"webbrain-companion/webbrain-companion", "new", 0o755})
	if err := Install(pkg, Target{Kind: "binary", Path: bin}); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(Target{Kind: "binary", Path: bin}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "old" {
		t.Fatalf("program after the rollback: %q", got)
	}
	if err := Rollback(Target{Kind: "binary", Path: bin}); err == nil {
		t.Fatal("a second rollback found something to put back")
	}
}

func TestCleanupAfterAnUpdate(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "webbrain-companion")
	_ = os.WriteFile(bin, []byte("new"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".webbrain-companion.old"), []byte("old"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, ".wbc-update-1"), 0o755)
	downloads := t.TempDir()
	_ = os.MkdirAll(filepath.Join(downloads, "0.1.3"), 0o755)
	_ = os.WriteFile(filepath.Join(downloads, "attempt.json"), []byte("{}"), 0o600)
	Cleanup(bin, downloads, "0.1.3")
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("the program's folder holds %v", names(entries))
	}
	if entries, _ := os.ReadDir(downloads); len(entries) != 1 || entries[0].Name() != "attempt.json" {
		t.Fatalf("downloads hold %v; files such as the attempt record stay", names(entries))
	}
}

func names(entries []os.DirEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
