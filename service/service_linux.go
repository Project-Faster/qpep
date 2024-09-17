package service

import (
	"github.com/Project-Faster/qpep/shared/logger"
	"os"
)

const (
	PLATFORM_EXE_NAME            = "qpep"
	PLATFORM_PATHVAR_SEP         = ':'
	PLATFORM_SERVICE_CLIENT_NAME = "qpep-client"
	PLATFORM_SERVICE_SERVER_NAME = "qpep-server"
)

// setCurrentWorkingDir method is currently a no-op
func setCurrentWorkingDir(path string) bool {
	return true // no-op
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
