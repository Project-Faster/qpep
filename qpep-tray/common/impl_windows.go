package common

import (
	"fmt"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/logger"
	"github.com/parvit/qpep/qpep-tray/icons"
	"github.com/parvit/qpep/qpep-tray/notify"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	EXENAME = "qpep.exe"

	CMD_SERVICE = `%s %s --service %s %s %s`
)

func getServiceCommand(start, client bool) *exec.Cmd {
	exeFile, _ := filepath.Abs(filepath.Join(ExeDir, EXENAME))

	var serviceFlag = "start"
	var clientFlag = "--client"
	var hostFlag = ""
	if client {
		hostFlag = fmt.Sprintf("-Dlistenaddress=%s", configuration.QPepConfig.Client.LocalListeningAddress)
	} else {
		hostFlag = fmt.Sprintf("-Dlistenaddress=%s", configuration.QPepConfig.Server.LocalListeningAddress)
	}
	var verboseFlag = "--verbose"
	if !start {
		serviceFlag = "stop"
	}
	if !client {
		verboseFlag = ""
	}
	if !configuration.QPepConfig.General.Verbose {
		verboseFlag = ""
	}

	attr := &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    fmt.Sprintf(CMD_SERVICE, exeFile, clientFlag, serviceFlag, hostFlag, verboseFlag),
	}

	cmd := exec.Command(exeFile)
	if cmd == nil {
		notify.ErrorMsg("Could not create client command")
		return nil
	}
	cmd.Dir, _ = filepath.Abs(ExeDir)
	cmd.SysProcAttr = attr
	return cmd
}
