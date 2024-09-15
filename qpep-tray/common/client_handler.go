package common

import (
	"errors"
	"github.com/Project-Faster/qpep/qpep-tray/notify"
	"github.com/Project-Faster/qpep/shared/configuration"
	stderr "github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/workers/gateway"
)

var clientActive bool = false

func startClient() error {
	if clientActive {
		logger.Error("Cannot start an already running client, first stop it")
		return stderr.ErrFailed
	}

	outAddress := configuration.QPepConfig.Client.LocalListeningAddress
	addressList, _ := gateway.GetLanListeningAddresses()
	for idx, addr := range addressCheckBoxList {
		if addr.Checked() {
			outAddress = addressList[idx]
			logger.Info("Forced Listening address to %v\n", outAddress)
			break
		}
	}

	override := configuration.QPepConfigType{
		Client: &configuration.ClientDefinition{
			LocalListeningAddress: outAddress,
			LocalListenPort:       configuration.QPepConfig.Client.LocalListenPort,
			GatewayHost:           configuration.QPepConfig.Client.GatewayHost,
			GatewayPort:           configuration.QPepConfig.Client.GatewayPort,
		},
	}

	configuration.WriteConfigurationOverrideFile(override)

	if err := startClientProcess(); err != nil {
		notify.ErrorMsg("Could not start client program: %v", err)
		clientActive = false
		return stderr.ErrCommandNotStarted
	}
	clientActive = true
	notify.InfoMsg("Client started")

	return nil
}

func stopClient() error {
	if !clientActive {
		return nil
	}

	if err := stopClientProcess(); err != nil {
		notify.ErrorMsg("Could not stop process gracefully (%v)n", err)
		return err
	}

	clientActive = false
	gateway.SetSystemProxy(false)
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
		logger.Error("Full command: %v", cmd.String())
		out, _ := cmd.CombinedOutput()
		logger.Error("Full error: %v", string(out))
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
