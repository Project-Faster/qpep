package client

import (
	"github.com/parvit/qpep/shared"
)

// initDiverter method wraps the logic for initializing the windiverter engine, returns true if the diverter
// succeeded initialization and false otherwise
func initDiverter() bool {
	return shared.SetConnectionDiverter(true, "", shared.QPepConfig.ListenHost, 0, shared.QPepConfig.ListenPort, 0, 0)
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {
	shared.SetConnectionDiverter(false, "", "", 0, 0, 0, 0)
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
