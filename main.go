package main

import (
	"github.com/parvit/qpep/service"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/shared/logger"
	"os"
	"runtime/debug"
)

func init() {
	logger.SetupLogger("qpep-service.log", "info")
}

func main() {
	//f, _ := os.Create("trace.out")
	//trace.Start(f)

	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v %v\n", err, string(debug.Stack()))
		}
		logger.CloseLogger()
	}()

	tsk := shared.StartRegion("ServiceMain")
	retcode := service.ServiceMain()
	tsk.End()

	logger.Info("=== EXIT - code(%d) ===", retcode)

	//trace.Stop()
	//f.Sync()
	//f.Close()

	os.Exit(retcode)
}
