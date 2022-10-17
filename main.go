package main

import (
	"runtime/debug"

	. "github.com/parvit/qpep/logger"
)

func init() {
	SetupLogger("qpep-service.log")
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v", err)
			debug.PrintStack()
		}
	}()

	serviceMain()

	Info("=== EXIT ===")
}
