//go:build !cgo
// +build !cgo

package windivert

import (
	. "github.com/parvit/qpep/logger"
	"log"
)

const (
	DIVERT_OK                  = 0
	DIVERT_ERROR_NOTINITILIZED = 1
	DIVERT_ERROR_ALREADY_INIT  = 2
	DIVERT_ERROR_FAILED        = 3
)

func InitializeWinDivertEngine(gatewayAddr, listenAddr string, gatewayPort, listenPort, numThreads int, gatewayInterfaces []int64) int {
	Info("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return DIVERT_OK
}

func CloseWinDivertEngine() int {
	Info("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return DIVERT_OK
}

func GetConnectionStateData(port int) (int, int, int, string, string) {
	Info("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return DIVERT_OK, -1, -1, "", ""
}

func EnableDiverterLogging(enable bool) {
	Info("WARNING: windivert package compiled without CGO") // message to check for failing CGO
	return
}
