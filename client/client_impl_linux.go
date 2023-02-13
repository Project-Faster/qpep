package client

import (
	"github.com/parvit/qpep/shared"
)

// initDiverter method wraps the logic for initializing the windiverter engine, returns true if the diverter
// succeeded initialization and false otherwise
func initDiverter() bool {
	return false
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {}

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
