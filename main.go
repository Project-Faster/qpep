package main

import (
	"github.com/Project-Faster/qpep/logger"
	"github.com/Project-Faster/qpep/service"
	"github.com/Project-Faster/qpep/shared"
	"os"
	"runtime/debug"
	"runtime/trace"
)

func init() {
	logger.SetupLogger("qpep-service.log")
}

func main() {
	f, _ := os.Create("trace.out")
	trace.Start(f)

	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v", err)
			debug.PrintStack()
		}
	}()

	tsk := shared.StartRegion("ServiceMain")
	retcode := service.ServiceMain()
	tsk.End()

	logger.Info("=== EXIT - code(%d) ===", retcode)
	logger.CloseLogger()

	trace.Stop()
	f.Sync()
	f.Close()

	os.Exit(retcode)
}
