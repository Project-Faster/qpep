package common

import (
	"errors"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/qpep-tray/notify"
	"github.com/parvit/qpep/shared"
)

var serverActive bool = false

func startServer() error {
	if serverActive {
		return shared.ErrFailed
	}

	addressList, _ := shared.GetLanListeningAddresses()
	for idx, addr := range addressCheckBoxList {
		if addr.Checked() {
			shared.QPepConfig.ListenHost = addressList[idx]
			logger.Error("Forced Listening address to %v\n", shared.QPepConfig.ListenHost)
			break
		}
	}

	shared.WriteConfigurationOverrideFile(map[string]string{
		"listenaddress": shared.QPepConfig.ListenHost,
	})

	if err := startServerProcess(); err != nil {
		notify.ErrorMsg("Could not start server program: %v", err)
		serverActive = false
		return shared.ErrCommandNotStarted
	}
	serverActive = true
	notify.InfoMsg("Server started")

	return nil
}

func stopServer() error {
	if !serverActive {
		return nil
	}

	if err := stopServerProcess(); err != nil {
		logger.Error("Could not stop process gracefully (%v)\n", err)
		return err
	}

	serverActive = false
	notify.InfoMsg("Server stopped")
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
