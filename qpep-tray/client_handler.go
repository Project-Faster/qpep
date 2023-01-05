package main

import (
	"log"
	"os/exec"

	"github.com/parvit/qpep/shared"
)

var clientCmd *exec.Cmd

func startClient() error {
	if clientCmd != nil {
		log.Println("ERROR: Cannot start an already running client, first stop it")
		return shared.ErrFailed
	}

	addressList, _ := shared.GetLanListeningAddresses()
	for idx, addr := range addressCheckBoxList {
		if addr.Checked() {
			qpepConfig.ListenHost = addressList[idx]
			log.Printf("Forced Listening address to %v\n", qpepConfig.ListenHost)
			break
		}
	}

	clientCmd = getClientCommand()

	if err := clientCmd.Start(); err != nil {
		ErrorMsg("Could not start client program: %v", err)
		clientCmd = nil
		return shared.ErrCommandNotStarted
	}
	InfoMsg("Client started")

	return nil
}

func stopClient() error {
	if clientCmd == nil {
		log.Println("ERROR: Cannot stop an already stopped client, first start it")
		return nil
	}

	if err := stopClientProcess(); err != nil {
		log.Printf("Could not stop process gracefully (%v), will try to force-terminate it\n", err)

		if err := clientCmd.Process.Kill(); err != nil {
			ErrorMsg("Could not force-terminate process")
			return err
		}
	}

	clientCmd.Wait()
	clientCmd = nil
	shared.SetSystemProxy(false)
	InfoMsg("Client stopped")
	return nil
}

func reloadClientIfRunning() {
	if clientCmd == nil {
		return
	}

	stopClient()
	startClient()
}
