package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/client"
	"github.com/parvit/qpep/server"
	"github.com/parvit/qpep/shared"

	"github.com/parvit/qpep/windivert"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("PANIC: %v", err)
			debug.PrintStack()
		}
		shared.SetSystemProxy(false) // be sure to clear proxy settings on exit
	}()

	log.SetFlags(log.Ltime | log.Lmicroseconds)

	shared.ParseFlags(os.Args) // don't skip first parameter

	logName := "qpep-server.log"
	if shared.QuicConfiguration.ClientFlag {
		logName = "qpep-client.log"
	}

	f, err := os.OpenFile(logName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	wrt := io.MultiWriter(os.Stdout, f)
	log.SetOutput(wrt)

	execContext, cancelExecutionFunc := context.WithCancel(context.Background())

	shared.SetSystemProxy(false) // clear previous data

	go api.RunServer(execContext, cancelExecutionFunc, true) // api server for local webgui

	if shared.QuicConfiguration.ClientFlag {
		runAsClient(execContext, cancelExecutionFunc)
	} else {
		runAsServer(execContext, cancelExecutionFunc)
	}

	interruptListener := make(chan os.Signal, 1)
	signal.Notify(interruptListener, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

TERMINATIONLOOP:
	for {
		select {
		case <-interruptListener:
			break TERMINATIONLOOP
		case <-execContext.Done():
			break TERMINATIONLOOP
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}

	cancelExecutionFunc()
	<-execContext.Done()

	log.Println("Shutdown...")
	log.Println(windivert.CloseWinDivertEngine())

	<-time.After(1 * time.Second)

	log.Println("Exiting...")
	os.Exit(1)
}

func runAsClient(execContext context.Context, cancel context.CancelFunc) {
	log.Println("Running Client")

	windivert.EnableDiverterLogging(shared.QuicConfiguration.Verbose)

	go client.RunClient(execContext, cancel)
}

func runAsServer(execContext context.Context, cancel context.CancelFunc) {
	log.Println("Running Server")
	go server.RunServer(execContext, cancel)
	go api.RunServer(execContext, cancel, false)
}
