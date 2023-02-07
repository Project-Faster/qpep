//go:build windows

// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package service

import (
	"bou.ke/monkey"
	"errors"
	"github.com/parvit/qpep/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestServiceWinSuite(t *testing.T) {
	var q ServiceWinSuite
	suite.Run(t, &q)
}

type ServiceWinSuite struct{ suite.Suite }

func (s *ServiceWinSuite) SetupSuite() {
	_ = os.Mkdir(".tempPath", 0777)
}

func (s *ServiceWinSuite) TearDownSuite() {
	_ = os.RemoveAll(".tempPath")
}

func (s *ServiceWinSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
}

func (s *ServiceWinSuite) BeforeTest(_, _ string) {}

func (s *ServiceWinSuite) TestSetCurrentWorkingDir() {
	oldPath, _ := os.Getwd()

	newPath := filepath.Join(oldPath, ".tempPath")
	assert.True(s.T(), setCurrentWorkingDir(newPath))

	currentPath, _ := os.Getwd()
	assert.Equal(s.T(), newPath, currentPath)

	assert.True(s.T(), setCurrentWorkingDir(oldPath))
}

func (s *ServiceWinSuite) TestSetCurrentWorkingDir_FailPathInvalid() {
	p, _ := os.Getwd()

	p = filepath.Join(p, ".temp\x00Path")
	assert.False(s.T(), setCurrentWorkingDir(p))

	newP, _ := os.Getwd()
	assert.NotEqual(s.T(), p, newP)
}

func (s *ServiceWinSuite) TestSetCurrentWorkingDir_FailPathNotExist() {
	p, _ := os.Getwd()

	p = filepath.Join(p, ".tempPathNotExist")
	assert.False(s.T(), setCurrentWorkingDir(p))

	newP, _ := os.Getwd()
	assert.NotEqual(s.T(), p, newP)
}

func (s *ServiceWinSuite) TestSetServiceUserPermissions() {
	monkey.Patch(shared.RunCommand, func(name string, params ...string) ([]byte, error) {
		assert.Equal(s.T(), "sc.exe", name)
		assert.Len(s.T(), params, 3)
		assert.Equal(s.T(), "sdset", params[0])
		assert.Equal(s.T(), "test-service", params[1])
		assert.Equal(s.T(), USER_ACCESS_LIST, params[2])
		return nil, nil
	})

	assert.NotPanics(s.T(), func() {
		setServiceUserPermissions("test-service")
	})
}

func (s *ServiceWinSuite) TestSetServiceUserPermissions_Error() {
	monkey.Patch(shared.RunCommand, func(string, ...string) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	assert.NotPanics(s.T(), func() {
		setServiceUserPermissions("test-service")
	})
}

func (s *ServiceWinSuite) TestInstallDirectoryPermissions() {
	monkey.Patch(shared.RunCommand, func(name string, params ...string) ([]byte, error) {
		assert.Equal(s.T(), "icacls", name)
		switch len(params) {
		case 5:
			assert.Equal(s.T(), ".", params[0])
			assert.Equal(s.T(), "/t", params[1])
			assert.Equal(s.T(), "/q", params[2])
			assert.Equal(s.T(), "/c", params[3])
			assert.Equal(s.T(), "/reset", params[4])
			break
		case 4:
			assert.Equal(s.T(), ".", params[0])
			assert.Equal(s.T(), "/t", params[1])
			assert.Equal(s.T(), "/grant", params[2])
			assert.Equal(s.T(), "Everyone:F", params[3])
			break
		default:
			assert.Failf(s.T(), "Number of parameters unexpected", "%d", len(params))
			return nil, errors.New("")
		}
		return nil, nil
	})

	assert.NotPanics(s.T(), func() {
		setInstallDirectoryPermissions(".\\.tempPath")
	})
}

func (s *ServiceWinSuite) TestInstallDirectoryPermissions_ErrorFirstCmd() {
	monkey.Patch(shared.RunCommand, func(string, ...string) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	assert.Panics(s.T(), func() {
		setInstallDirectoryPermissions("test-service")
	})
}

func (s *ServiceWinSuite) TestInstallDirectoryPermissions_ErrorSecondCmd() {
	monkey.Patch(shared.RunCommand, func(_ string, params ...string) ([]byte, error) {
		if params[len(params)-1] == "/reset" {
			return nil, nil
		}
		return nil, errors.New("test-error")
	})

	assert.Panics(s.T(), func() {
		setInstallDirectoryPermissions("test-service")
	})
}

func (s *ServiceWinSuite) TestSendProcessInterrupt() {
	var proxyWasReset = false
	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		if !active {
			proxyWasReset = true
		}
	})

	routeCmd := exec.Command("cmd.exe", "/c", `timeout /t 30`)
	routeCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	assert.Nil(s.T(), routeCmd.Start())

	monkey.Patch(os.Getpid, func() int {
		return routeCmd.Process.Pid
	})

	<-time.After(1 * time.Second)

	sendProcessInterrupt()

	assert.True(s.T(), proxyWasReset)
}

func (s *ServiceWinSuite) TestSendProcessInterrupt_ErrorKill() {
	var proxyWasReset = false
	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		if !active {
			proxyWasReset = true
		}
	})

	monkey.Patch(os.Getpid, func() int {
		return -1
	})

	<-time.After(1 * time.Second)

	sendProcessInterrupt()

	assert.True(s.T(), proxyWasReset)
}

func (s *ServiceWinSuite) TestWaitChildProcessTermination() {
	var start = time.Now()

	waitChildProcessTermination("go")

	list, err := processes("go")
	assert.Nil(s.T(), err)

	assert.LessOrEqual(s.T(), len(list), 1)

	assert.LessOrEqual(s.T(), time.Now(), start.Add(30*time.Second))
}
