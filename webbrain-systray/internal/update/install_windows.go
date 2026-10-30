//go:build windows

package update

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// uninstallKey is where packaging/windows/installer.nsi records the
// installation.
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\WebBrainCompanion`

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}

// installedHere checks that exe is the copy the setup program installed, which
// the silent setup replaces.
func installedHere(exe string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return errors.New("this copy was not installed with the setup program; download the new version by hand")
	}
	defer k.Close()
	loc, _, err := k.GetStringValue("InstallLocation")
	if err != nil || loc == "" {
		return errors.New("this copy was not installed with the setup program; download the new version by hand")
	}
	dir := filepath.Dir(exe)
	if !strings.EqualFold(filepath.Clean(loc), filepath.Clean(dir)) {
		return fmt.Errorf("WebBrain Companion is installed in %s, but this copy runs from %s; download the new version by hand", loc, dir)
	}
	// The silent setup runs without administrator rights; a folder it cannot
	// write (an installation made as administrator) would make it fail.
	return writable(dir)
}

// Exec is how macOS and Linux start an installed update; on Windows the setup
// program starts it.
func Exec(string, ...string) error { return errors.New("not used on Windows") }

// StartInstaller runs the setup program silently in update mode: it waits for
// process pid (this companion, which quits next) to exit, installs over the
// registered installation and starts the new version.
func StartInstaller(setup string, pid int) error {
	cmd := exec.Command(setup, "/S", "/UPDATE="+strconv.Itoa(pid))
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// StartSetup opens the setup program with its usual pages, for an update
// started from the command line.
func StartSetup(setup string) error {
	cmd := exec.Command(setup)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
