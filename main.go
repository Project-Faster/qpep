package main

import (
	"github.com/parvit/qpep/service"
	"os"
	"runtime/debug"

	"github.com/parvit/qpep/logger"
)

func init() {
	logger.SetupLogger("qpep-service.log")
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v", err)
			debug.PrintStack()
		}
	}()

	retcode := service.ServiceMain()

	logger.Info("=== EXIT - code(%d) ===", retcode)
	logger.CloseLogger()

	os.Exit(retcode)
}
