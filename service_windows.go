package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	. "github.com/parvit/qpep/logger"
	"golang.org/x/sys/windows"
)

const (
	USER_ACCESS_LIST = `D:(A;;CCLCSWRPWPDTLOCRRC;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)(A;;CCLCSWLOCRRC;;;IU)(A;;CCLCSWLOCRRC;;;SU)(A;;RPWPCR;;;%s)S:(AU;FA;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;WD)`
)

func setCurrentWorkingDir(path string) {
	imagePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		Error("ERROR: %v\n", err)
		os.Exit(1)
	}

	if err = windows.SetCurrentDirectory(imagePath); err != nil {
		Error("ERROR: %v\n", err)
		os.Exit(1)
	}
}

func setServiceUserPermissions(serviceName string) {
	// Extract UserID of user, works even if running with admin rights
	userIdCmd := exec.Command("whoami", "/user", "/fo", "csv", "/nh")
	userIdCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := userIdCmd.CombinedOutput()
	if err != nil {
		Error("%v", err)
		panic(err)
	}

	// example output:
	// "laptop-acer\acer","S-1-5-21-4227727717-1300533570-3298936513-1001"
	lines := strings.Split(string(output), ",")
	userID := strings.TrimSpace(strings.Replace(lines[1], `"`, ``, -1))

	// use the extracted UserID to allow start / stop privileges to user
	// without admin rights
	cmdParams := []string{
		`sdset`,
		serviceName,
		fmt.Sprintf(USER_ACCESS_LIST, userID),
	}

	permssionsCmd := exec.Command(`sc.exe`, cmdParams...)
	permssionsCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	_, err = permssionsCmd.CombinedOutput()
	if err != nil {
		Error("%v", err)
		panic(err)
	}

	Info("Service installed correctly")
}

func setInstallDirectoryPermissions(installDir string) {
	// reset access permission on the installation directory to allow writing logs
	cmdParams := []string{
		installDir,
		`/t`, `/q`, `/c`, `/reset`,
	}

	permssionsCmd := exec.Command(`icacls`, cmdParams...)
	permssionsCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	_, err := permssionsCmd.CombinedOutput()
	if err != nil {
		Error("%v", err)
		panic(err)
	}

	// allow users group to access installation directory to allow writing logs
	cmdParams = []string{
		installDir,
		`/grant`, `Users:F`,
	}

	permssionsCmd = exec.Command(`icacls`, cmdParams...)
	permssionsCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	_, err = permssionsCmd.CombinedOutput()
	if err != nil {
		Error("%v", err)
		panic(err)
	}
}

func sendProcessInterrupt() {
	dll := syscall.MustLoadDLL("kernel32.dll")
	defer func() {
		if dll != nil {
			dll.Release()
		}
	}()

	p, err := dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		Error("ERROR: %v\n", err)
		os.Exit(1)
	}

	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms683155(v=vs.85).aspx
	pid := os.Getpid()
	r1, _, err := p.Call(syscall.CTRL_BREAK_EVENT, uintptr(pid))
	if r1 == 0 {
		Error("ERROR: %v\n", err)
		os.Exit(1)
	}
}

func waitChildProcessTermination(name string) {
	count := 0
	for timeout := 30; timeout > 0; timeout-- {
		count = 0

		list, err := processes()
		if err != nil {
			for _, p := range list {
				if strings.EqualFold(p.Exe, name) {
					count++
				}
			}
			if count < 2 {
				return // only this process remains
			}
		}

		<-time.After(1 * time.Second)
	}
}

const TH32CS_SNAPPROCESS = 0x00000002

type windowsProcess struct {
	ProcessID       int
	ParentProcessID int
	Exe             string
}

func processes() ([]windowsProcess, error) {
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

	results := make([]windowsProcess, 0, 50)
	for {
		results = append(results, newWindowsProcess(&entry))

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

func newWindowsProcess(e *windows.ProcessEntry32) windowsProcess {
	// Find when the string ends for decoding
	end := 0
	for {
		if e.ExeFile[end] == 0 {
			break
		}
		end++
	}

	return windowsProcess{
		ProcessID:       int(e.ProcessID),
		ParentProcessID: int(e.ParentProcessID),
		Exe:             syscall.UTF16ToString(e.ExeFile[:end]),
	}
}