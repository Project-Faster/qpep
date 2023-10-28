package common

import (
	"os/exec"
	"path/filepath"

	"github.com/Project-Faster/qpep/qpep-tray/notify"

	"github.com/Project-Faster/qpep/shared"
)

const (
	EXENAME = "qpep"

	CMD_SERVICE = `%s -service %s %s %s`
)

func getServiceCommand(start, client bool) *exec.Cmd {
	exeFile, _ := filepath.Abs(filepath.Join(ExeDir, EXENAME))

	var serviceFlag = "start"
	var clientFlag = "--client"
	var verboseFlag = "--verbose"
	if !start {
		serviceFlag = "stop"
	}
	if !client {
		verboseFlag = ""
	}
	if !shared.QPepConfig.Verbose {
		verboseFlag = ""
	}

	cmd := exec.Command(exeFile, clientFlag,
		"--service", serviceFlag,
		"-Dlistenaddress", shared.QPepConfig.ListenHost,
		verboseFlag)
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
