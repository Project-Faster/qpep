package main

import (
	"github.com/Project-Faster/qpep/qpep-tray/common"
	"github.com/Project-Faster/qpep/qpep-tray/notify"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Project-Faster/qpep/shared"
	"github.com/getlantern/systray"
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

	// open the log file in current directory
	f, err := os.OpenFile(filepath.Join(common.ExeDir, "qpep-tray.log"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	wrt := io.MultiWriter(os.Stdout, f)
	log.SetOutput(wrt)

	log.SetFlags(log.Ltime | log.Lmicroseconds)

	// read configuration
	if err := shared.ReadConfiguration(true); err != nil {
		notify.ErrorMsg("Could not load configuration file, please edit: %v", err)
	}

	systray.Run(common.OnReady, common.OnExit)
}
