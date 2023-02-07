package service

import (
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// USER_ACCESS_LIST respresents an ACL Security Descriptor that allows non-admin users (actually the Everyone group)
	// to be able to start and stop services while allowing the actual service to run with elevated privileges
	// One can use `ConvertFrom-SddlString -Sddl "<sd-descriptor>"` in a powershell shell to see the contents of the descriptor
	USER_ACCESS_LIST = `D:(A;;CCLCSWRPWPDTLOCRRC;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)(A;;CCLCSWLOCRRC;;;IU)(A;;CCLCSWLOCRRC;;;SU)(A;;RPWPCR;;;S-1-1-0)S:(AU;FA;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;WD)`
)

// setCurrentWorkingDir method allows to change the working directory of the current executable, because the default
// working directory for windows services is C:\Windows\System32
func setCurrentWorkingDir(path string) bool {
	logger.Info("Set working path %s", path)
	imagePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		logger.Error("%v\n", err)
		return false
	}

	if err = windows.SetCurrentDirectory(imagePath); err != nil {
		logger.Error("%v\n", err)
		return false
	}
	return true
}

// setServiceUserPermissions runs on the "install" command and allows the just installed service indicated by the _serviceName_
// parameter to be started / stopped by a non-admin user (by setting the acl sd on the service), this works because the
// install command in necessarily given when the user has administrator privileges
func setServiceUserPermissions(serviceName string) {
	logger.Info("Set user permissions %s", serviceName)
	logger.Debug(`acl: "%s"`, USER_ACCESS_LIST)

	_, err := shared.RunCommand(`sc.exe`, `sdset`, serviceName, USER_ACCESS_LIST)
	if err != nil {
		logger.Error("%v", err)
		return
	}
}

// setInstallDirectoryPermissions runs on the "install" command and sets the permissions of the installation directory
// so that logs and configuration files can be written into it
func setInstallDirectoryPermissions(workDir string) {
	installDir := filepath.Dir(workDir) // Permissions should be set for the entire folder

	logger.Info("Set install permissions %s", installDir)

	// reset access permission on the installation directory to allow writing logs
	_, err := shared.RunCommand(`icacls`, installDir, `/t`, `/q`, `/c`, `/reset`)
	if err != nil {
		logger.Panic("%v", err)
	}

	// allow Everyone group to access installation directory to allow writing logs
	_, err = shared.RunCommand(`icacls`, installDir, `/t`, `/grant`, `Everyone:F`)
	if err != nil {
		logger.Panic("%v", err)
	}
}

// sendProcessInterrupt method wraps the windows-specific logic for generating a ctrl-break signal to a service
// so that it can be interrupted gracefully
func sendProcessInterrupt() {
	logger.Debug("sendProcessInterrupt")

	// disable proxy
	shared.SetSystemProxy(false)

	dll := syscall.MustLoadDLL("kernel32.dll")
	defer func() {
		if dll != nil {
			dll.Release()
		}
	}()

	p, _ := dll.FindProc("GenerateConsoleCtrlEvent")

	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms683155(v=vs.85).aspx
	pid := os.Getpid()
	r1, _, err := p.Call(syscall.CTRL_BREAK_EVENT, uintptr(pid))
	if r1 == 0 {
		logger.Error("%v\n", err)
		return
	}
}

// waitChildProcessTermination loops the process list for 30 seconds to wait for child processes to terminate correctly
func waitChildProcessTermination(name string) {
	logger.Info("Wait child terminate %s", name)

	for timeout := 30; timeout > 0; timeout-- {
		list, err := processes(name)
		if len(list) < 2 || err != nil {
			return // only this process remains
		}

		<-time.After(1 * time.Second)
	}
}

// TH32CS_SNAPPROCESS is the code for creating a copy of the running process list
const TH32CS_SNAPPROCESS = 0x00000002

// windowsProcess struct models an handle to a windows executing process
type windowsProcess struct {
	ProcessID       int
	ParentProcessID int
	Exe             string
}

// returns a list of all windows processes with the given name
func processes(name string) ([]windowsProcess, error) {
	handle, err := windows.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(handle)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	// get the first process
	err = windows.Process32First(handle, &entry)
	if err != nil {
		return nil, err
	}

	results := make([]windowsProcess, 0, 64)
	for {
		p := newWindowsProcess(&entry)
		if strings.EqualFold(p.Exe, name) {
			results = append(results, p)
		}

		err = windows.Process32Next(handle, &entry)
		if err != nil {
			// windows sends ERROR_NO_MORE_FILES on last process
			if err == syscall.ERROR_NO_MORE_FILES {
				return results, nil
			}
			return nil, err
		}
	}
}

// newWindowsProcess convert the found windows process list entry to an usable
// windowsProcess struct
func newWindowsProcess(e *windows.ProcessEntry32) windowsProcess {
	// Find when the string ends for decoding
	end := 0
	for ; e.ExeFile[end] != 0 && end < len(e.ExeFile); end++ {
		// no-op
	}

	return windowsProcess{
		ProcessID:       int(e.ProcessID),
		ParentProcessID: int(e.ParentProcessID),
		Exe:             syscall.UTF16ToString(e.ExeFile[:end]),
	}
}
