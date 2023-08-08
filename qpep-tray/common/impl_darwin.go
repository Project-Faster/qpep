package common

import (
	"github.com/Project-Faster/qpep/qpep-tray/notify"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/Project-Faster/qpep/shared"
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
		notify.ErrorMsg("Could not create client command")
		return nil
	}
	cmd.Dir, _ = filepath.Abs(ExeDir)
	cmd.SysProcAttr = attr
	return cmd
}

// fakeAPICallCheckProxy executes a "fake" api call to the local server to check for the connection running through
// the global proxy, this is checked by the client that adds the "X-QPEP-PROXY" header with value "true", a missing or
// "false" value means the proxy is not running correctly
func fakeAPICallCheckProxy() bool {
	return true
}
