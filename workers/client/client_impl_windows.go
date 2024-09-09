package client

import (
	"github.com/parvit/qpep/shared"
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

	return shared.SetConnectionDiverter(true, gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID)
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {
	shared.SetConnectionDiverter(false, "", "", 0, 0, 0, 0)
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
