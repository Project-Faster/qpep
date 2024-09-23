//go:build windows && cgo

package windivert

//#cgo windows CPPFLAGS: -DWIN32 -D_WIN32_WINNT=0x0600 -I ${SRCDIR}/include/
//#cgo windows,amd64 LDFLAGS: ${SRCDIR}/x64/WinDivert.dll
//#cgo windows,386 LDFLAGS: ${SRCDIR}/x86/WinDivert.dll
//#include "windivert_wrapper.h"
import "C"

import (
	"github.com/parvit/qpep/shared"
	"time"
	"unsafe"

	. "github.com/parvit/qpep/logger"
)

func init() {
	_, _, code := shared.RunCommand("sc.exe", "queryex", "WinDivert") // Check orphaned instances of WinDivert
	if code != 0 {
		return
	}

	_, _, _ = shared.RunCommand("sc.exe", "stop", "WinDivert") // Stop orphaned instances of WinDivert
	<-time.After(1 * time.Second)
	_, _, code = shared.RunCommand("sc.exe", "queryex", "WinDivert") // Check orphaned instances of WinDivert
	if code != 0 {
		return
	}

	panic("Tried to stop the WinDivert orphan instance but it did not terminate, unable to continue")
}

// InitializeWinDivertEngine method invokes the initialization of the WinDivert library, specifying that:
// * _gatewayAddr_ Packets must be redirected to this address
// * _listenAddr_ Packets must have source on this address
// * _gatewayPort_ Packets must be redirected to this port
// * _listenPort_ Packets must have source from this port
// * _numThreads_ Number of threads to use for the packet capturing routines
// * _gatewayInterfaces_ Only accept divert of packets of this interface id
// * _portRanges_ List of ports to ignore
func InitializeWinDivertEngine(gatewayAddr, listenAddr string,
	gatewayPort, listenPort, numThreads int,
	gatewayInterface int64,
	ignoredPorts []int) int {

	ports := make([]C.int, 0, len(ignoredPorts))
	ports = append(ports, C.int(53)) // DNS

	for i := 0; i < len(ignoredPorts); i++ {
		if ignoredPorts[i] < 0 || ignoredPorts[i] >= 65536 {
			return DIVERT_ERROR_FAILED
		}
		ports = append(ports, C.int(ignoredPorts[i]))
	}

	gatewayStr := C.CString(gatewayAddr)
	listenStr := C.CString(listenAddr)
	response := int(C.InitializeWinDivertEngine(gatewayStr, listenStr, C.int(gatewayPort), C.int(listenPort), C.int(numThreads), (*C.int)(&ports[0]), C.int(len(ports))))
	if response != DIVERT_OK {
		return response
	}

	C.SetGatewayInterfaceIndexToDivert(C.int(gatewayInterface))
	return response
}

// CloseWinDivertEngine method closes a running WinDivert engine
func CloseWinDivertEngine() int {
	return int(C.CloseWinDivertEngine())
}

// GetConnectionStateData method returns the data for a connection on the specified port:
// * error code
// * source port
// * destination port
// * source address
// * destination address
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

// EnableDiverterLogging method sets to active or not the verbose logging of the windivert library
// !! Warning !! Activating this incurs in heavy performance cost (mostly in the C<->Go context switch
// for logging to the go stream)
func EnableDiverterLogging(enable bool) {
	val := 0
	msg := "Diverter debug messages won't be output"
	if enable {
		msg = "Diverter debug messages will be output"
		val = 1
	}

	Info(msg)
	C.EnableMessageOutputToGo(C.int(val))
}

//export logMessageToGo
func logMessageToGo(msg *C.char) {
	Info(C.GoString(msg))
}
