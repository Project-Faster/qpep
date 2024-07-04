package common

import (
	"errors"
	"github.com/Project-Faster/qpep/qpep-tray/notify"
	"log"

	"github.com/Project-Faster/qpep/shared"
)

var clientActive bool = false

func startClient() error {
	if clientActive {
		log.Println("ERROR: Cannot start an already running client, first stop it")
		return shared.ErrFailed
	}

	addressList, _ := shared.GetLanListeningAddresses()
	for idx, addr := range addressCheckBoxList {
		if addr.Checked() {
			shared.QPepConfig.ListenHost = addressList[idx]
			log.Printf("Forced Listening address to %v\n", shared.QPepConfig.ListenHost)
			break
		}
	}

	shared.WriteConfigurationOverrideFile(map[string]string{
		"listenaddress": shared.QPepConfig.ListenHost,
	})

	if err := startClientProcess(); err != nil {
		notify.ErrorMsg("Could not start client program: %v", err)
		clientActive = false
		return shared.ErrCommandNotStarted
	}
	clientActive = true
	notify.InfoMsg("Client started")

	return nil
}

func stopClient() error {
	if !clientActive {
		notify.ErrorMsg("ERROR: Cannot stop an already stopped client, first start it")
		return nil
	}

	if err := stopClientProcess(); err != nil {
		notify.ErrorMsg("Could not stop process gracefully (%v)n", err)
		return err
	}

	clientActive = false
	shared.SetSystemProxy(false)
	notify.InfoMsg("Client stopped")
	return nil
}

func reloadClientIfRunning() {
	if !clientActive {
		return
	}

	stopClient()
	startClient()
}

func startClientProcess() error {
	cmd := getServiceCommand(true, true)
	if cmd == nil {
		return errors.New("Failed command")
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	err = cmd.Wait()
	if err != nil {
		log.Printf("ERR: Full command: %v", cmd.String())
		out, _ := cmd.CombinedOutput()
		log.Printf("ERR: Full error: %v", string(out))
	}
	return err
}

func stopClientProcess() error {
	cmd := getServiceCommand(false, true)
	if cmd == nil {
		return errors.New("Failed command")
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}
