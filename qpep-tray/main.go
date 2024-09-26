package main

import (
	"github.com/parvit/qpep/qpep-tray/common"
	"github.com/parvit/qpep/qpep-tray/notify"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/workers/gateway"
	"os"
	"os/signal"
	"syscall"

	"github.com/project-faster/systray"
)

func main() {
	defer func() {
		// clear the proxy in case a orphaned client cannot
		gateway.SetSystemProxy(false)
	}()

	// note: channel is never dequeued as to stop the ctrl-c signal from stopping also
	// this process and only the child client / server
	interruptListener := make(chan os.Signal, 1)
	signal.Notify(interruptListener, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	common.Init(os.Args[0])

	// read configuration
	if err := configuration.ReadConfiguration(true); err != nil {
		notify.ErrorMsg("Could not load configuration file, please edit: %v", err)
	}

	systray.Run(common.OnReady, common.OnExit)
}
