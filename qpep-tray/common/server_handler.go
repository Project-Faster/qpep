package common

import (
	"errors"
	"github.com/parvit/qpep/qpep-tray/notify"
	"github.com/parvit/qpep/shared/configuration"
	stderr "github.com/parvit/qpep/shared/errors"
	"github.com/parvit/qpep/shared/logger"
	"github.com/parvit/qpep/workers/gateway"
)

var serverActive bool = false

func startServer() error {
	if serverActive {
		return stderr.ErrFailed
	}

	outAddress := configuration.QPepConfig.Server.LocalListeningAddress
	addressList, _ := gateway.GetLanListeningAddresses()
	for idx, addr := range addressCheckBoxList {
		if addr.Checked() {
			outAddress = addressList[idx]
			logger.Error("Forced Listening address to %v\n", outAddress)
			break
		}
	}

	override := configuration.QPepConfigType{
		Server: &configuration.ServerDefinition{
			LocalListeningAddress: outAddress,
			LocalListenPort:       configuration.QPepConfig.Server.LocalListenPort,
		},
	}
	configuration.WriteConfigurationOverrideFile(override)

	if err := startServerProcess(); err != nil {
		notify.ErrorMsg("Could not start server program: %v", err)
		serverActive = false
		return stderr.ErrCommandNotStarted
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
