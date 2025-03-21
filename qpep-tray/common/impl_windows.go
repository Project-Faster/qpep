package common

import (
	"fmt"
	"github.com/Project-Faster/qpep/qpep-tray/icons"
	"github.com/Project-Faster/qpep/qpep-tray/notify"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/logger"
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

// fakeAPICallCheckProxy executes a "fake" api call to the local server to check for the connection running through
// the global proxy, this is checked by the client that adds the "X-QPEP-PROXY" header with value "true", a missing or
// "false" value means the proxy is not running correctly
func fakeAPICallCheckProxy(host string) bool {
	data, err, _ := shared.RunCommand("powershell.exe",
		"-WindowStyle", "hidden",
		"-ExecutionPolicy", "ByPass", "-Command",
		fmt.Sprintf("Invoke-WebRequest -Uri \"http://%s:444/qpep-client-proxy-check\" -UseBasicParsing -TimeoutSec 1", host),
	)
	logger.Info("proxy check data: %s", data)
	logger.Info("proxy check error: %v", err)
	if err != nil {
		return false
	}
	if strings.Contains(string(data), "X-QPEP-PROXY, true") {
		logger.Info("proxy is working")
		return true
	}
	return false
}

func getWaitingIcons() [][]byte {
	return [][]byte{
		icons.MainIconWaiting,
		icons.MainIconData,
	}
}
