package logger

import (
	"os"
	"path/filepath"

	stdlog "log"

	"github.com/davecgh/go-spew/spew"
	"github.com/parvit/qpep/flags"
	"github.com/parvit/qpep/version"
	log "github.com/rs/zerolog"
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

	/*quicFile := filepath.Join(filepath.Dir(execPath), "quic.log")
	fQuic, err := os.OpenFile(quicFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		stdlog.Fatalf("%v", err)
	}
	stdlog.SetOutput(fQuic)*/

	log.SetGlobalLevel(log.InfoLevel)

	_log = log.New(f).Level(log.DebugLevel).
		With().Timestamp().Logger()

	Info("=== QPep version %s ===", version.Version())
	Info(spew.Sdump(flags.Globals))
}

func Info(format string, values ...interface{}) {
	_log.Info().Msgf(format, values...)
}

func Debug(format string, values ...interface{}) {
	_log.Debug().Msgf(format, values...)
}

func Error(format string, values ...interface{}) {
	_log.Error().Msgf(format, values...)
}
