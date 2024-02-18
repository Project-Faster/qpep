/*
Package logger provides a very basic interface to logging throughout the project.

By default, it logs to standard out and when SetupLogger is called it outputs to a file
and on windows it also outputs to OutputDebugString facility if level is debug.

The level is set using the global log level of package zerolog.
*/
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nyaosorg/go-windows-dbg"
	log "github.com/rs/zerolog"
)

// _log customized logger instance
var _log log.Logger

// _logFile customized logger output file
var _logFile *os.File //

func init() {
	CloseLogger()
}

// getLoggerFile Sets up a new logging file overwriting the previous one if found
func getLoggerFile(logName string) *os.File {
	execPath, err := os.Executable()
	if err != nil {
		Panic("Could not find executable: %s", err)
	}

	logFile := filepath.Join(filepath.Dir(execPath), logName)

	_logFile, err = os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		Panic("%v", err)
	}
	return _logFile
}

// SetupLogger Sets up a new logger destroying the previous one to a file with name "qpep_<logName>.log"
func SetupLogger(logName string, level string) {
	CloseLogger()

	_logFile = getLoggerFile(logName)

	logLevel, err := log.ParseLevel(level)
	if err != nil {
		logLevel = log.InfoLevel
	}

	log.SetGlobalLevel(logLevel)
	log.TimeFieldFormat = time.StampMilli

	_log = log.New(io.MultiWriter(_logFile, os.Stdout)).
		With().Logger()
}

// CloseLogger Terminates the current log and resets it to stdout output
func CloseLogger() {
	if _logFile == nil {
		return
	}
	_ = _logFile.Sync()
	_ = _logFile.Close()
	_logFile = nil

	_log = log.New(os.Stdout)
}

// GetLogger allows external libraries to integrate with the qpep logger
func GetLogger() *log.Logger {
	return &_log
}

// Info Outputs a new formatted string with the provided parameters to the logger instance with Info level
// Outputs the same data to the OutputDebugString facility if os is Windows and level is set to Debug
func Info(format string, values ...interface{}) {
	_log.Info().Time("time", time.Now()).Msgf(format, values...)
	//stdlog.Printf(format, values...)
	if runtime.GOOS == "windows" && _log.GetLevel() >= log.DebugLevel {
		_, _ = dbg.Printf(format, values...)
		return
	}
}

// Debug Outputs a new formatted string with the provided parameters to the logger instance with Debug level
// Outputs the same data to the OutputDebugString facility if os is Windows and level is set to Debug
func Debug(format string, values ...interface{}) {
	if log.GlobalLevel() != log.DebugLevel {
		return
	}
	_log.Debug().Time("time", time.Now()).Msgf(format, values...)
	//stdlog.Printf(format, values...)
	if runtime.GOOS == "windows" && _log.GetLevel() >= log.DebugLevel {
		_, _ = dbg.Printf(format, values...)
		return
	}
}

// Error Outputs a new formatted string with the provided parameters to the logger instance with Error level
// Outputs the same data to the OutputDebugString facility if os is Windows and level is set to Debug
func Error(format string, values ...interface{}) {
	_log.Error().Time("time", time.Now()).Msgf(format, values...)
	//stdlog.Printf(format, values...)
	if runtime.GOOS == "windows" && _log.GetLevel() >= log.DebugLevel {
		_, _ = dbg.Printf(format, values...)
		return
	}
}

// Panic Outputs a new formatted string with the provided parameters to the logger instance with Error level
// Outputs the same data to the OutputDebugString facility if os is Windows and level is set to Debug
// and then panics with the same formatted string
func Panic(format string, values ...interface{}) {
	_log.Error().Time("time", time.Now()).Msgf(format, values...)
	if runtime.GOOS == "windows" && _log.GetLevel() >= log.DebugLevel {
		_, _ = dbg.Printf(format, values...)
	}
	panic(fmt.Sprintf(format, values...))
}

func Trace() {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		Info("[trace][%s:%d]", "<missing>", 0)
		return
	}
	Info("[trace][%s:%d]", file, line)
}

// OnError method sends an error log only if the err value in input is not nil
func OnError(err error, msg string) {
	if err == nil {
		return
	}
	Error("error %v "+msg, err.Error())
}
