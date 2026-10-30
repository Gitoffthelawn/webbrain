package update

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/esokullu/webbrain-systray/internal/download"
)

// Target is what an update replaces for the running copy.
type Target struct {
	// Kind is "bundle" (the macOS app), "binary" (the Linux program) or
	// "installer" (Windows, where the setup program installs over the
	// registered installation).
	Kind string
	// Path is the .app bundle or the executable.
	Path string
}

// ErrTranslocated: macOS runs the app from a temporary read-only copy because
// it was opened where it was downloaded.
var ErrTranslocated = errors.New("macOS runs this copy from a temporary location; move WebBrain Companion to Applications and open it from there")

// bundleName and binaryName are the files scripts/build.sh packages.
const (
	bundleName = "WebBrain Companion.app"
	bundleExe  = "WebBrain Companion"
	binaryName = "webbrain-companion"
	stagingPre = ".wbc-update-"
)

// Locate finds what to replace for the executable exe, or why this copy cannot
// update itself.
func Locate(exe string) (Target, error) { return locate(exe, runtime.GOOS) }

func locate(exe, goos string) (Target, error) {
	switch goos {
	case "darwin":
		if strings.Contains(exe, "/AppTranslocation/") {
			return Target{}, ErrTranslocated
		}
		bundle := bundleOf(exe)
		if bundle == "" {
			return Target{}, errors.New("this copy does not run from an app bundle; download the new version by hand")
		}
		if err := writable(filepath.Dir(bundle)); err != nil {
			return Target{}, err
		}
		return Target{Kind: "bundle", Path: bundle}, nil
	case "linux":
		if err := writable(filepath.Dir(exe)); err != nil {
			return Target{}, err
		}
		return Target{Kind: "binary", Path: exe}, nil
	case "windows":
		if err := installedHere(exe); err != nil {
			return Target{}, err
		}
		return Target{Kind: "installer", Path: exe}, nil
	}
	return Target{}, fmt.Errorf("no WebBrain Companion package exists for %s", goos)
}

// bundleOf returns the app bundle around exe (…/X.app/Contents/MacOS/x), or "".
func bundleOf(exe string) string {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	bundle := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(bundle, ".app") {
		return ""
	}
	return bundle
}

// writable checks that files can be created in dir, where the update swaps
// the old copy for the new one.
func writable(dir string) error {
	f, err := os.CreateTemp(dir, stagingPre+"check-")
	if err != nil {
		return fmt.Errorf("cannot write to %s, so this copy cannot replace itself; download the new version by hand", dir)
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

// Install puts the package at pkg in place of t. The swap happens in the same
// folder, so the running process keeps its old files until it exits. On
// Windows use StartInstaller instead.
func Install(pkg string, t Target) error {
	switch t.Kind {
	case "bundle":
		return installBundle(pkg, t.Path)
	case "binary":
		return installBinary(pkg, t.Path)
	}
	return fmt.Errorf("a %q target is updated by its setup program", t.Kind)
}

// oldBundle is where the replaced app waits until the new version has
// started and removes it (Cleanup).
func oldBundle(bundle string) string {
	return filepath.Join(filepath.Dir(bundle), "."+filepath.Base(bundle)+".old")
}

func installBundle(zipPath, bundle string) error {
	if err := checkZip(zipPath); err != nil {
		return err
	}
	parent := filepath.Dir(bundle)
	stage, err := os.MkdirTemp(parent, stagingPre)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	keep := func(rel string) bool { return strings.HasPrefix(rel, bundleName+"/") }
	if err := download.ExtractZip(zipPath, stage, 0, keep); err != nil {
		return fmt.Errorf("unpack the new version: %w", err)
	}
	fresh := filepath.Join(stage, bundleName)
	exe := filepath.Join(fresh, "Contents", "MacOS", bundleExe)
	if st, err := os.Lstat(exe); err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
		return errors.New("the downloaded app is incomplete")
	}
	if st, err := os.Lstat(filepath.Join(fresh, "Contents", "Info.plist")); err != nil || !st.Mode().IsRegular() {
		return errors.New("the downloaded app is incomplete")
	}
	old := oldBundle(bundle)
	_ = os.RemoveAll(old)
	if err := os.Rename(bundle, old); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			// macOS 13 and later guard apps against changes by other apps.
			return fmt.Errorf("macOS did not let WebBrain Companion replace itself; allow it under System Settings, Privacy & Security, App Management, or download the new version by hand (%w)", err)
		}
		return fmt.Errorf("move the running app aside: %w", err)
	}
	if err := os.Rename(fresh, bundle); err != nil {
		if restoreErr := os.Rename(old, bundle); restoreErr != nil {
			return errors.Join(fmt.Errorf("put the new app in place: %w", err), restoreErr)
		}
		return fmt.Errorf("put the new app in place: %w", err)
	}
	return nil
}

// checkZip refuses archives with links: the extractor writes plain files, and
// a bundle that relies on links would come out broken.
func checkZip(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("the package contains a link (%s); install this version by hand", f.Name)
		}
	}
	return nil
}

func installBinary(tarPath, exe string) error {
	stage, err := os.MkdirTemp(filepath.Dir(exe), stagingPre)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	keep := func(rel string) bool { return rel == binaryName }
	if err := download.ExtractTarGz(tarPath, stage, 1, keep); err != nil {
		return fmt.Errorf("unpack the new version: %w", err)
	}
	fresh := filepath.Join(stage, binaryName)
	st, err := os.Lstat(fresh)
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return errors.New("the downloaded program is incomplete")
	}
	if err := os.Chmod(fresh, 0o755); err != nil {
		return err
	}
	// The running process keeps its file (the inode) whatever the name; the
	// old program stays next to it until the new version has started.
	old := oldBinary(exe)
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("move the running program aside: %w", err)
	}
	if err := os.Rename(fresh, exe); err != nil {
		if restoreErr := os.Rename(old, exe); restoreErr != nil {
			return errors.Join(fmt.Errorf("put the new program in place: %w", err), restoreErr)
		}
		return fmt.Errorf("put the new program in place: %w", err)
	}
	return nil
}

// oldBinary is where the replaced Linux program waits until the new version
// has started.
func oldBinary(exe string) string {
	return filepath.Join(filepath.Dir(exe), "."+filepath.Base(exe)+".old")
}

// Rollback puts back the version an Install replaced, for when the new one
// does not start.
func Rollback(t Target) error {
	switch t.Kind {
	case "bundle":
		old := oldBundle(t.Path)
		if _, err := os.Stat(old); err != nil {
			return fmt.Errorf("the previous app is gone: %w", err)
		}
		failed := filepath.Join(filepath.Dir(t.Path), "."+filepath.Base(t.Path)+".failed")
		_ = os.RemoveAll(failed)
		if err := os.Rename(t.Path, failed); err != nil {
			return err
		}
		if err := os.Rename(old, t.Path); err != nil {
			return errors.Join(err, os.Rename(failed, t.Path))
		}
		return os.RemoveAll(failed)
	case "binary":
		return os.Rename(oldBinary(t.Path), t.Path)
	}
	return fmt.Errorf("a %q target has nothing to put back", t.Kind)
}

// Cleanup removes what an earlier update left: the replaced app bundle
// (macOS) or program (Linux), unfinished staging folders next to exe, and
// downloaded packages (one folder per version) that are not newer than
// current.
func Cleanup(exe, downloads, current string) {
	dir := filepath.Dir(exe)
	if bundle := bundleOf(exe); bundle != "" {
		_ = os.RemoveAll(oldBundle(bundle))
		_ = os.RemoveAll(filepath.Join(filepath.Dir(bundle), "."+filepath.Base(bundle)+".failed"))
		dir = filepath.Dir(bundle)
	} else if filepath.Base(exe) == binaryName {
		_ = os.Remove(oldBinary(exe))
	}
	if matches, err := filepath.Glob(filepath.Join(dir, stagingPre+"*")); err == nil {
		for _, m := range matches {
			_ = os.RemoveAll(m)
		}
	}
	entries, _ := os.ReadDir(downloads)
	for _, e := range entries {
		if e.IsDir() && !Newer(e.Name(), current) {
			_ = os.RemoveAll(filepath.Join(downloads, e.Name()))
		}
	}
}
