//go:build !cgo

package windivert

import (
	. "github.com/parvit/qpep/logger"
	"log"
)

// InitializeWinDivertEngine method invokes the initialization of the WinDivert library, specifying that:
// * _gatewayAddr_ Packets must be redirected to this address
// * _listenAddr_ Packets must have source on this address
// * _gatewayPort_ Packets must be redirected to this port
// * _listenPort_ Packets must have source from this port
// * _numThreads_ Number of threads to use for the packet capturing routines
// * _gatewayInterfaces_ Only accept divert of packets of this interface id
func InitializeWinDivertEngine(gatewayAddr, listenAddr string, gatewayPort, listenPort, numThreads int, gatewayInterfaces int64) int {
	log.Println("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return DIVERT_OK
}

// CloseWinDivertEngine method closes a running WinDivert engine
func CloseWinDivertEngine() int {
	Info("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return DIVERT_OK
}

// GetConnectionStateData method returns the data for a connection on the specified port:
// * error code
// * source port
// * destination port
// * source address
// * destination address
func GetConnectionStateData(port int) (int, int, int, string, string) {
	Info("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return DIVERT_OK, -1, -1, "", ""
}

// EnableDiverterLogging method sets to active or not the verbose logging of the windivert library
// !! Warning !! Activating this incurs in heavy performance cost (mostly in the C<->Go context switch
// for logging to the go stream)
func EnableDiverterLogging(enable bool) {
	Info("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return
}
