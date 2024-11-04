package client

import (
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/workers/gateway"
)

// initDiverter method wraps the logic for initializing the windiverter engine, returns true if the diverter
// succeeded initialization and false otherwise
func initDiverter() bool {
	clientConfig := configuration.QPepConfig.Client
	filteredPorts := configuration.QPepConfig.Limits.IgnoredPorts

	listenPort := clientConfig.LocalListenPort
	listenHost := clientConfig.LocalListeningAddress

	redirected = gateway.SetConnectionDiverter(true, "",
		listenHost, 0,
		listenPort, 0,
		0, filteredPorts)

	return redirected
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {
	redirected = false
	gateway.SetConnectionDiverter(false, "", "", 0, 0, 0, 0, []int{})
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
