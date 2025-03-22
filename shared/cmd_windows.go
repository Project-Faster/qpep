package shared

import (
	"fmt"
	"github.com/Project-Faster/qpep/shared/logger"
	"golang.org/x/sys/windows"
	"os/exec"
	"strings"
	"syscall"
)

// RunCommand method abstracts the execution of a system command and returns the combined stdout,stderr streams and
// an error if there was any issue with the command executed
func RunCommand(name string, cmd ...string) ([]byte, error, int) {
	realCmd := fmt.Sprintf("%s %s", name, strings.Join(cmd, " "))
	logger.Debug(realCmd)

	// add wrapper parameters
	cmd = append([]string{"/c", name}, cmd...)
	cmd = append(cmd, "&&", "exit")

	routeCmd := exec.Command("cmd.exe", cmd...)
	routeCmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	result, err := routeCmd.CombinedOutput()
	code := routeCmd.ProcessState.ExitCode()

	return result, err, code
}
