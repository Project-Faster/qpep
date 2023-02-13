package main

import (
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/parvit/qpep/shared"
)

const (
	EXENAME = "qpep"

	CMD_SERVICE = `%s -service %s %s %s`
)

func getServiceCommand(start, client bool) *exec.Cmd {
	exeFile, _ := filepath.Abs(filepath.Join(ExeDir, EXENAME))

	var serviceFlag = "start"
	var clientFlag = "-client"
	var verboseFlag = "-verbose"
	if !start {
		serviceFlag = "stop"
	}
	if !client {
		verboseFlag = ""
	}
	if !shared.QPepConfig.Verbose {
		verboseFlag = ""
	}

	attr := &syscall.SysProcAttr{}

	cmd := exec.Command(exeFile, serviceFlag, clientFlag, verboseFlag)
	if cmd == nil {
		ErrorMsg("Could not create client command")
		return nil
	}
	cmd.Dir, _ = filepath.Abs(ExeDir)
	cmd.SysProcAttr = attr
	return cmd
}
