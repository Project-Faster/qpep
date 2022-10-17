package main

import (
	"errors"
	"log"

	"github.com/parvit/qpep/shared"
)

var serverActive bool = false

func startServer() error {
	if serverActive {
		log.Println("ERROR: Cannot start an already running server, first stop it")
		return shared.ErrFailed
	}

	if err := startServerProcess; err != nil {
		ErrorMsg("Could not start server program: %v", err)
		serverActive = false
		return shared.ErrCommandNotStarted
	}
	serverActive = true
	InfoMsg("Server started")

	return nil
}

func stopServer() error {
	if !serverActive {
		log.Println("ERROR: Cannot stop an already stopped server, first start it")
		return nil
	}

	if err := stopServerProcess(); err != nil {
		log.Printf("Could not stop process gracefully (%v)\n", err)
		return err
	}

	serverActive = false
	InfoMsg("Server stopped")
	return nil
}

func reloadServerIfRunning() {
	if !serverActive {
		return
	}

	stopServer()
	startServer()
}

func startServerProcess() error {
	cmd := getServiceCommand(true, false)
	if cmd == nil {
		return errors.New("Failed command")
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}
func stopServerProcess() error {
	cmd := getServiceCommand(false, false)
	if cmd == nil {
		return errors.New("Failed command")
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}
