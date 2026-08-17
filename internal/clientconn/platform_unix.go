//go:build unix

package clientconn

import (
	"errors"
	"os"
	"syscall"
)

func validatePrivateFile(info os.FileInfo) error {
	if info.Mode().Perm() != 0o600 {
		return errors.New("permissions must be 0600")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("owner must be the current user")
	}
	return nil
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
