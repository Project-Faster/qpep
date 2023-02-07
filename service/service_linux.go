package service

import (
	"github.com/parvit/qpep/logger"
	"os"
)

// setCurrentWorkingDir method is currently a no-op
func setCurrentWorkingDir(path string) {
	return // no-op
}

// sendProcessInterrupt method send an interrupt signal to the service
func sendProcessInterrupt() {
	pid := os.Getpid()
	p, err := os.FindProcess(pid)
	if err != nil {
		logger.Error("%v\n", err)
		return
	}
	if err = p.Signal(os.Interrupt); err != nil {
		logger.Error("%v\n", err)
		return
	}
}

// waitChildProcessTermination method is currently a no-op
func waitChildProcessTermination(name string) {
	return
}

// setServiceUserPermissions method is currently a no-op
func setServiceUserPermissions(serviceName string) {
	return
}

// setInstallDirectoryPermissions method is currently a no-op
func setInstallDirectoryPermissions(installDir string) {
	return
}
