//go:build !windows

package update

import (
	"errors"
	"os"
	"syscall"
)

// Exec replaces this process with exe. The process keeps its ID, session and
// (under systemd) its unit, so nothing outside notices a new program; files
// this process opened close, which also releases the single-instance lock. It
// returns only when exe could not be started.
func Exec(exe string, args ...string) error {
	return syscall.Exec(exe, append([]string{exe}, args...), os.Environ())
}

func installedHere(string) error { return errors.New("the setup program exists only on Windows") }

// StartInstaller runs the Windows setup program; other systems install in
// place (Install).
func StartInstaller(string, int) error {
	return errors.New("the setup program exists only on Windows")
}

// StartSetup opens the Windows setup program; other systems install in place.
func StartSetup(string) error { return errors.New("the setup program exists only on Windows") }
