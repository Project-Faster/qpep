package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Project-Faster/qpep/shared/logger"
)

const (
	PLATFORM_EXE_NAME            = "qpep"
	PLATFORM_PATHVAR_SEP         = ':'
	PLATFORM_SERVICE_CLIENT_NAME = "com.project-faster.qpep-client"
	PLATFORM_SERVICE_SERVER_NAME = "com.project-faster.qpep-server"
)

// RunCommand method abstracts the execution of a system command and returns the combined stdout,stderr streams and
// an error if there was any issue with the command executed
func runCommand(name string, cmd ...string) ([]byte, error, int) {
	routeCmd := exec.Command(name, cmd...)
	routeCmd.SysProcAttr = &syscall.SysProcAttr{}
	result, err := routeCmd.CombinedOutput()
	code := routeCmd.ProcessState.ExitCode()

	return result, err, code
}

// setCurrentWorkingDir method is currently a no-op
func setCurrentWorkingDir(path string) bool {
	return os.Chdir(path) == nil
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
	pid := os.Getpid()
	p, _ := os.FindProcess(pid)
	for p != nil {
		p, _ = os.FindProcess(pid)
		<-time.After(10 * time.Millisecond)
	}
}

func setServiceUserPermissions(serviceName string) {
	user := os.Getenv("USER")
	serviceFile := fmt.Sprintf("/Users/%s/Library/LaunchAgents/%s.plist", user, serviceName)
	out, _, _ := runCommand("sudo", "-A", "chown", "root:wheel", serviceFile)
	fmt.Printf("service ownership: %s\n", string(out))
	out, _, _ = runCommand("sudo", "-A", "chmod", "o-w", serviceFile)
	fmt.Printf("service permission: %s\n", string(out))
}

// setInstallDirectoryPermissions method is currently a no-op
func setInstallDirectoryPermissions(installDir string) {
	_ = os.Mkdir(filepath.Join(installDir, "log"), 0755)
	return
}
