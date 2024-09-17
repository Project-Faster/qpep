package common

import (
	"github.com/Project-Faster/qpep/qpep-tray/icons"
	"github.com/Project-Faster/qpep/qpep-tray/notify"
	"github.com/Project-Faster/qpep/shared/configuration"
	"os/exec"
	"path/filepath"
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

func fakeAPICallCheckProxy() bool {
	return true
}

func getWaitingIcons() [][]byte {
	return [][]byte{
		icons.MainIconWaiting,
		icons.MainIconWaiting2,
	}
}
