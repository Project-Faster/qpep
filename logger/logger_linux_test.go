//go:build !windows

// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package logger

import (
	"bou.ke/monkey"
	"errors"
	log "github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerSuite(t *testing.T) {
	var q LoggerSuite
	suite.Run(t, &q)
}

type LoggerSuite struct{ suite.Suite }

var testerr = errors.New("test-error")

func (s *LoggerSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
}

func (s *LoggerSuite) BeforeTest(_, _ string) {}

func (s *LoggerSuite) TestCloseLogger() {
	SetupLogger("test", "info")

	var prevlog = _log
	CloseLogger()
	assert.NotEqual(s.T(), _log, prevlog)
}

func (s *LoggerSuite) TestLogger_InfoLevel() {
	t := s.T()
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test", "info")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, _log.GetLevel(), log.InfoLevel)
	assert.Equal(t, log.GlobalLevel(), log.InfoLevel)

	Info("InfoMessage")
	Debug("DebugMessage")
	Error("ErrorMessage")
	OnError(nil, "OnErrorEmpty")
	OnError(testerr, "OnErrorPresent")

	data, _ := os.ReadFile(logFile)
	var strData = string(data)
	assert.NotEqual(t, -1, strings.Index(strData, "InfoMessage"))
	assert.Equal(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
	assert.Equal(t, -1, strings.Index(strData, "OnErrorEmpty"))
	assert.NotEqual(t, -1, strings.Index(strData, "OnErrorPresent"))
}

func (s *LoggerSuite) TestLogger_DebugLevel() {
	t := s.T()
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test", "debug")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, log.DebugLevel, _log.GetLevel())
	assert.Equal(t, log.DebugLevel, log.GlobalLevel())

	log.SetGlobalLevel(log.DebugLevel)

	Info("InfoMessage")
	Debug("DebugMessage")
	Error("ErrorMessage")
	OnError(nil, "OnErrorEmpty")
	OnError(testerr, "OnErrorPresent")

	data, _ := os.ReadFile(logFile)
	var strData = string(data)
	assert.NotEqual(t, -1, strings.Index(strData, "InfoMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
	assert.Equal(t, -1, strings.Index(strData, "OnErrorEmpty"))
	assert.NotEqual(t, -1, strings.Index(strData, "OnErrorPresent"))
}

func (s *LoggerSuite) TestLogger_ErrorLevel() {
	t := s.T()
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test", "info")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, _log.GetLevel(), log.InfoLevel)
	assert.Equal(t, log.GlobalLevel(), log.InfoLevel)

	log.SetGlobalLevel(log.ErrorLevel)

	Info("InfoMessage")
	Debug("DebugMessage")
	Error("ErrorMessage")
	OnError(nil, "OnErrorEmpty")
	OnError(testerr, "OnErrorPresent")

	data, _ := os.ReadFile(logFile)
	var strData = string(data)
	assert.Equal(t, -1, strings.Index(strData, "InfoMessage"))
	assert.Equal(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
	assert.Equal(t, -1, strings.Index(strData, "OnErrorEmpty"))
	assert.NotEqual(t, -1, strings.Index(strData, "OnErrorPresent"))
}

func (s *LoggerSuite) TestLogger_PanicMessage() {
	t := s.T()
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test", "info")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, _log.GetLevel(), log.InfoLevel)
	assert.Equal(t, log.GlobalLevel(), log.InfoLevel)

	log.SetGlobalLevel(log.DebugLevel)

	Info("InfoMessage")
	assert.PanicsWithValue(t, "PanicMessage", func() {
		Panic("PanicMessage")
	})
	Debug("DebugMessage")
	Error("ErrorMessage")

	data, _ := os.ReadFile(logFile)
	var strData = string(data)
	assert.NotEqual(t, -1, strings.Index(strData, "InfoMessage"))
	assert.Equal(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "PanicMessage"))
}

func (s *LoggerSuite) TestLogger_OutputDebugString_DebugLevel() {
	t := s.T()
	SetupLogger("test", "info")

	log.SetGlobalLevel(log.DebugLevel)

	var counter = 0
	guard := monkey.Patch(dbg.Printf, func(format string, values ...interface{}) (int, error) {
		counter++
		_ = fmt.Sprint(fmt.Sprintf(format, values...))
		return 0, nil
	})
	defer func() {
		if guard != nil {
			guard.Restore()
		}
	}()

	Info("InfoMessage")
	assert.PanicsWithValue(t, "PanicMessage", func() {
		Panic("PanicMessage")
	})
	Debug("DebugMessage")
	Error("ErrorMessage")

	assert.Equal(t, 4, counter)
}

func (s *LoggerSuite) TestLogger_getLoggerFileFailExecutable() {
	t := s.T()

	var guard *monkey.PatchGuard
	guard = monkey.Patch(os.OpenFile, func(name string, flag int, perm os.FileMode) (*os.File, error) {
		if !strings.Contains(name, "invalid") {
			guard.Unpatch()
			defer guard.Restore()

			return os.OpenFile(name, flag, perm)
		}
		return nil, errors.New("file not found")
	})
	assert.Panics(t, func() {
		getLoggerFile("test invalid/")
	})
	if guard != nil {
		guard.Restore()
	}
}

func (s *LoggerSuite) TestLogger_getLoggerFileFailOpenFile() {
	t := s.T()

	guard := monkey.Patch(os.Executable, func() (string, error) {
		return "", errors.New("file not found")
	})
	defer func() {
		if guard != nil {
			guard.Restore()
		}
	}()
	assert.Panics(t, func() {
		getLoggerFile("exec not found/")
	})
}
