package clientconn

import (
	"os"
	"syscall"
)

func validatePrivateFile(_ os.FileInfo) error {
	return nil
}

func processAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	return syscall.CloseHandle(handle) == nil
}
