package common

import (
	"github.com/parvit/qpep/qpep-tray/icons"
	"github.com/parvit/qpep/qpep-tray/notify"
	"github.com/parvit/qpep/shared/configuration"
	"os/exec"
	"path/filepath"
)

const (
	EXENAME = "qpep"
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
	if !configuration.QPepConfig.General.Verbose {
		verboseFlag = ""
	}

	cmd := exec.Command(exeFile, "-service", serviceFlag, clientFlag, verboseFlag)
	if cmd == nil {
		notify.ErrorMsg("Could not create client command")
		return nil
	}
	cmd.Dir, _ = filepath.Abs(ExeDir)
	return cmd
}

// fakeAPICallCheckProxy executes a "fake" api call to the local server to check for the connection running through
// the global proxy, this is checked by the client that adds the "X-QPEP-PROXY" header with value "true", a missing or
// "false" value means the proxy is not running correctly
func fakeAPICallCheckProxy() bool {
	return true
}

func getWaitingIcons() [][]byte {
	return [][]byte{
		icons.MainIconWaiting,
		icons.MainIconData,
	}
}
