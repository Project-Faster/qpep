package main

import (
	"github.com/parvit/qpep/qpep-tray/common"
	"github.com/parvit/qpep/qpep-tray/notify"
	"os"
	"os/signal"
	"syscall"

	"github.com/parvit/qpep/shared"
	"github.com/project-faster/systray"
)

func main() {
	defer func() {
		// clear the proxy in case a orphaned client cannot
		shared.SetSystemProxy(false)
	}()

	// note: channel is never dequeued as to stop the ctrl-c signal from stopping also
	// this process and only the child client / server
	interruptListener := make(chan os.Signal, 1)
	signal.Notify(interruptListener, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	common.Init(os.Args[0])

	// read configuration
	if err := shared.ReadConfiguration(true); err != nil {
		notify.ErrorMsg("Could not load configuration file, please edit: %v", err)
	}

	systray.Run(common.OnReady, common.OnExit)
}
