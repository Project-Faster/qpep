//go:build !windows

// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package logger

import (
	"errors"
	log "github.com/rs/zerolog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bou.ke/monkey"
	"github.com/stretchr/testify/assert"
)

func TestLogger_InfoLevel(t *testing.T) {
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, _log.GetLevel(), log.DebugLevel)
	assert.Equal(t, log.GlobalLevel(), log.InfoLevel)

	Info("InfoMessage")
	Debug("DebugMessage")
	Error("ErrorMessage")

	data, _ := os.ReadFile(logFile)
	var strData = string(data)
	assert.NotEqual(t, -1, strings.Index(strData, "InfoMessage"))
	assert.Equal(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
}

func TestLogger_DebugLevel(t *testing.T) {
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, _log.GetLevel(), log.DebugLevel)
	assert.Equal(t, log.GlobalLevel(), log.InfoLevel)

	log.SetGlobalLevel(log.DebugLevel)

	Info("InfoMessage")
	Debug("DebugMessage")
	Error("ErrorMessage")

	data, _ := os.ReadFile(logFile)
	var strData = string(data)
	assert.NotEqual(t, -1, strings.Index(strData, "InfoMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
}

func TestLogger_ErrorLevel(t *testing.T) {
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, _log.GetLevel(), log.DebugLevel)
	assert.Equal(t, log.GlobalLevel(), log.InfoLevel)

	log.SetGlobalLevel(log.ErrorLevel)

	Info("InfoMessage")
	Debug("DebugMessage")
	Error("ErrorMessage")

	data, _ := os.ReadFile(logFile)
	var strData = string(data)
	assert.Equal(t, -1, strings.Index(strData, "InfoMessage"))
	assert.Equal(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
}

func TestLogger_PanicMessage(t *testing.T) {
	execPath, _ := os.Executable()

	logFile := filepath.Join(filepath.Dir(execPath), "test")

	var prevlog = _log
	SetupLogger("test")

	assert.NotEqual(t, prevlog, _log)
	assert.Equal(t, _log.GetLevel(), log.DebugLevel)
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
	assert.NotEqual(t, -1, strings.Index(strData, "DebugMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "ErrorMessage"))
	assert.NotEqual(t, -1, strings.Index(strData, "PanicMessage"))
}

func TestLogger_getLoggerFileFailExecutable(t *testing.T) {
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

func TestLogger_getLoggerFileFailOpenFile(t *testing.T) {
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
