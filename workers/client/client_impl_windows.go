package client

import (
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/workers/gateway"
	"net"
	"strings"
)

// initDiverter method wraps the logic for initializing the windiverter engine, returns true if the diverter
// succeeded initialization and false otherwise
func initDiverter() bool {
	generalConfig := configuration.QPepConfig.General
	clientConfig := configuration.QPepConfig.Client
	filteredPorts := configuration.QPepConfig.Limits.IgnoredPorts
	filteredPorts := shared.QPepConfig.IgnoredPorts

	gatewayHost := clientConfig.GatewayHost
	gatewayPort := clientConfig.GatewayPort
	listenPort := clientConfig.LocalListenPort
	listenHost := clientConfig.LocalListeningAddress
	threads := generalConfig.WinDivertThreads
	redirectedInterfaces := clientAdditional.RedirectedInterfaces

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

	redirected = gateway.SetConnectionDiverter(true, gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID, filteredPorts)
	return redirected
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {
	gateway.SetConnectionDiverter(false, "", "", 0, 0, 0, 0, []int{})
	redirected = false
}

// initProxy method wraps the calls for initializing the proxy
func initProxy() {
	gateway.UsingProxy = true
	gateway.SetSystemProxy(true)
}

// stopProxy method wraps the calls for stopping the proxy
func stopProxy() {
	redirected = false
	gateway.UsingProxy = false
	gateway.SetSystemProxy(false)
}
