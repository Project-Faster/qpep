package logger

import (
	"os"
	"path/filepath"
	"runtime"

	stdlog "log"

	log "github.com/rs/zerolog"

	"github.com/nyaosorg/go-windows-dbg"
)

var _log log.Logger

func init() {
	_log = log.New(os.Stdout)
}

func SetupLogger(logName string) {
	execPath, err := os.Executable()
	if err != nil {
		Error("Could not find executable: %s", err)
	}

	logFile := filepath.Join(filepath.Dir(execPath), logName)

	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		stdlog.Fatalf("%v", err)
	}

	log.SetGlobalLevel(log.InfoLevel)

	_log = log.New(f).Level(log.DebugLevel).
		With().Timestamp().Logger()
}

func Info(format string, values ...interface{}) {
	_log.Info().Msgf(format, values...)
	if runtime.GOOS == "windows" && _log.GetLevel() >= log.DebugLevel {
		_, _ = dbg.Printf(format, values...)
	}
}

func Debug(format string, values ...interface{}) {
	_log.Debug().Msgf(format, values...)
	if runtime.GOOS == "windows" && _log.GetLevel() >= log.DebugLevel {
		_, _ = dbg.Printf(format, values...)
	}
}

func Error(format string, values ...interface{}) {
	_log.Error().Msgf(format, values...)
	if runtime.GOOS == "windows" && _log.GetLevel() >= log.DebugLevel {
		_, _ = dbg.Printf(format, values...)
	}
}
