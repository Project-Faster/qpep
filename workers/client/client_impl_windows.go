package client

import (
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"net"
	"strings"
)

// initDiverter method wraps the logic for initializing the windiverter engine, returns true if the diverter
// succeeded initialization and false otherwise
func initDiverter() bool {
	gatewayHost := ClientConfiguration.GatewayHost
	gatewayPort := ClientConfiguration.GatewayPort
	listenPort := ClientConfiguration.ListenPort
	threads := ClientConfiguration.WinDivertThreads
	listenHost := ClientConfiguration.ListenHost
	redirectedInterfaces := ClientConfiguration.RedirectedInterfaces
	filteredPorts := shared.QPepConfig.IgnoredPorts

	// select an appropriate interface
	var redirectedInetID int64 = 0
	for _, id := range redirectedInterfaces {
		inet, _ := net.InterfaceByIndex(int(id))

		addresses, _ := inet.Addrs()
		for _, addr := range addresses {
			if strings.Contains(addr.String(), listenHost) {
				redirectedInetID = id
				break
			}
		}
	}

	logger.Info("WinDivert: %v %v %v %v %v %v %v\n", gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID, filteredPorts)
	code := windivert.InitializeWinDivertEngine(gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID, filteredPorts)
	logger.Info("WinDivert code: %v\n", code)
	if code != windivert.DIVERT_OK {
		logger.Panic("Could not initialize WinDivert engine, code %d\n", code)
	}
	return code == windivert.DIVERT_OK
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {
	windivert.CloseWinDivertEngine()
	redirected = false
}

// initProxy method wraps the calls for initializing the proxy
func initProxy() {
	shared.UsingProxy = true
	shared.SetSystemProxy(true)
}

// stopProxy method wraps the calls for stopping the proxy
func stopProxy() {
	redirected = false
	shared.UsingProxy = false
	shared.SetSystemProxy(false)
}
