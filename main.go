package main

import (
	"context"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/service"
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
		trace.Stop()
		f.Sync()
		f.Close()
	}()

	_, tsk := trace.NewTask(context.Background(), "ServiceMain")
	retcode := service.ServiceMain()
	tsk.End()

	logger.Info("=== EXIT - code(%d) ===", retcode)
	logger.CloseLogger()

	os.Exit(retcode)
}
