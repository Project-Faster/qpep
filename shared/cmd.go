package shared

import (
	"github.com/Project-Faster/qpep/shared/logger"
	"os/exec"
	"strings"
	"syscall"
)

// RunCommand method abstracts the execution of a system command and returns the combined stdout,stderr streams and
// an error if there was any issue with the command executed
func RunCommand(name string, cmd ...string) ([]byte, error, int) {
	logger.Debug("%s %s", name, strings.Join(cmd, " "))

	routeCmd := exec.Command(name, cmd...)
	routeCmd.SysProcAttr = &syscall.SysProcAttr{}
	result, err := routeCmd.CombinedOutput()
	code := routeCmd.ProcessState.ExitCode()

	return result, err, code
}
