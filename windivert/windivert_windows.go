//go:build windows && cgo

package windivert

//#cgo windows CPPFLAGS: -DWIN32 -D_WIN32_WINNT=0x0600 -I include/
//#cgo windows,amd64 LDFLAGS: windivert/x64/WinDivert.dll
//#cgo windows,386 LDFLAGS: windivert/x86/WinDivert.dll
//#include "windivert_wrapper.h"
import "C"

import (
	"unsafe"

	. "github.com/parvit/qpep/logger"
)

const (
	DIVERT_OK                  = 0
	DIVERT_ERROR_NOTINITILIZED = 1
	DIVERT_ERROR_ALREADY_INIT  = 2
	DIVERT_ERROR_FAILED        = 3
)

func InitializeWinDivertEngine(gatewayAddr, listenAddr string, gatewayPort, listenPort, numThreads int, gatewayInterfaces []int64) int {
	gatewayStr := C.CString(gatewayAddr)
	listenStr := C.CString(listenAddr)
	response := int(C.InitializeWinDivertEngine(gatewayStr, listenStr, C.int(gatewayPort), C.int(listenPort), C.int(numThreads)))
	if response != DIVERT_OK {
		return response
	}

	for _, idx := range gatewayInterfaces {
		C.AddGatewayInterfaceIndexToDivert(C.int(idx))
	}
	return response
}

func CloseWinDivertEngine() int {
	return int(C.CloseWinDivertEngine())
}

func GetConnectionStateData(port int) (int, int, int, string, string) {
	const n = C.sizeof_char

	var origSrcPort C.uint
	var origDstPort C.uint
	var origSrcAddress *C.char
	var origDstAddress *C.char

	origSrcAddress = (*C.char)(C.malloc(C.ulonglong(n) * C.ulonglong(65)))
	origDstAddress = (*C.char)(C.malloc(C.ulonglong(n) * C.ulonglong(65)))
	defer func() {
		_ = recover()
		C.free(unsafe.Pointer(origSrcAddress))
		C.free(unsafe.Pointer(origDstAddress))
	}()

	result := C.GetConnectionData(C.uint(port), &origSrcPort, &origDstPort, origSrcAddress, origDstAddress)
	if result == C.DIVERT_OK {
		return DIVERT_OK, int(origSrcPort), int(origDstPort), C.GoString(origSrcAddress), C.GoString(origDstAddress)
	}
	return int(result), -1, -1, "", ""
}

func EnableDiverterLogging(enable bool) {
	if enable {
		Info("Diverter messages will be output")
		C.EnableMessageOutputToGo(C.int(1))
	} else {
		Info("Diverter messages will be ignored")
		C.EnableMessageOutputToGo(C.int(0))
	}
}

//export logMessageToGo
func logMessageToGo(msg *C.char) {
	Info(C.GoString(msg))
}
