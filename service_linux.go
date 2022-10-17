package main

import (
	. "github.com/parvit/qpep/logger"
	"log"
	"os"
)

func setCurrentWorkingDir(path string) {
	return // no-op
}

func sendProcessInterrupt() {
	pid := os.Getpid()
	if p, err := os.FindProcess(pid); err != nil {
		Info("ERROR: %v\n", err)
		os.Exit(1)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		Info("ERROR: %v\n", err)
		os.Exit(1)
	}
}

func waitChildProcessTermination(name string) {
	return // TODO
}

func setServiceUserPermissions(serviceName string) {
	return // TODO
}

func setInstallDirectoryPermissions(installDir string) {
	return // TODO
}
