package client

import (
	"github.com/Project-Faster/qpep/workers/gateway"
)

// initDiverter method wraps the logic for initializing the windiverter engine, returns true if the diverter
// succeeded initialization and false otherwise
func initDiverter() bool {
	return true
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {
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
