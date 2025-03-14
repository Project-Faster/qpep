package gateway

import (
	"errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/jackpal/gateway"
)

var (
	// UsingProxy indicates globally to the program if the system proxy is being used instead of diverter
	UsingProxy = false
	// ProxyAddress is the address being used currently set as the system proxy
	ProxyAddress *url.URL

	// defaultListeningAddress is the address detected automatically by the system as default for listening locally
	defaultListeningAddress string
	// detectedGatewayInterfaces is the list of automatically detected network interfaces available in the system
	detectedGatewayInterfaces []int64
	// detectedGatewayAddresses is the list of automatically detected addresses available in the system (one per network device)
	detectedGatewayAddresses []string

	defaultListeningAddressList []string

	// timeoutLatencyMultiplier multiples the various timeouts to try to adapt to progressively higher latencies
	timeoutLatencyMultiplier int64 = 1
)

const (
	MAX_TIMEOUT_MULTIPLER = 16
	QPEP_PROXY_HEADER     = "X-QPEP-PROXY"
)

var (
	errNotImplemented = errors.New("not implemented for OS: " + runtime.GOOS)
)

// init method executes the static initialization for detecting the current system's interfaces and addresses
func init() {
	var err error
	detectedGatewayInterfaces, detectedGatewayAddresses, err = getRouteGatewayInterfaces()
	if err != nil {
		panic(err)
	}

	defaultListeningAddressList, _ = GetLanListeningAddresses()
}

// GetDefaultLanListeningAddress method allows the caller to obtain an address and a list of interfaces usable
// for listening on the network of current system, it takes into account the detected interfaces and addresses defined
// on the system and the preferred configuration address and gateway in input
func GetDefaultLanListeningAddress(currentAddress, gatewayAddress string) (string, []int64) {
	if len(currentAddress) > 0 {
		if !strings.HasPrefix(currentAddress, "0.") && !strings.HasPrefix(currentAddress, "127.") {
			return currentAddress, detectedGatewayInterfaces
		}
	}

	if len(defaultListeningAddress) > 0 {
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	var filterList = detectedGatewayAddresses

	if len(gatewayAddress) == 0 {
		defaultIP, err := gateway.DiscoverInterface()
		if err == nil {
			defaultListeningAddress = defaultIP.String()
			logger.Info("Found default ip address: %s\n", defaultListeningAddress)
			return defaultListeningAddress, detectedGatewayInterfaces
		}

		if err.Error() != errNotImplemented.Error() {
			logger.Panic("Could not discover default lan address and the requested one is not suitable, error: %v", err)
		}

		filterList = defaultListeningAddressList
	}

	logger.Info("WARNING: Detected invalid listening ip address, trying to autodetect the default route...\n")

	searchIdx := -1
	foundLongest := 0
	gatewaySplit := strings.Split(gatewayAddress, ".")

	for i := 0; i < len(filterList); i++ {
		addrComponents := strings.Split(filterList[i], ".")
		if addrComponents[0] != gatewaySplit[0] {
			continue
		}
		if addrComponents[1] != gatewaySplit[1] {
			if foundLongest == 0 {
				foundLongest = 1
				searchIdx = i
			}
			continue
		}
		if addrComponents[2] != gatewaySplit[2] {
			if foundLongest <= 1 {
				foundLongest = 2
				searchIdx = i
			}
			continue
		}
		if addrComponents[3] != gatewaySplit[3] {
			if foundLongest <= 2 {
				foundLongest = 2
				searchIdx = i
			}
			continue
		}
		searchIdx = i
		break
	}
	if searchIdx != -1 {
		defaultListeningAddress = filterList[searchIdx]
		logger.Info("Found default ip address: %s\n", defaultListeningAddress)
		return defaultListeningAddress, detectedGatewayInterfaces
	}
	defaultListeningAddress = filterList[0]
	return defaultListeningAddress, detectedGatewayInterfaces
}

// GetLanListeningAddresses returns all detected addresses and interfaces that can be used for listening
func GetLanListeningAddresses() ([]string, []int64) {
	defaultListeningAddressList = getRouteListeningAddresses()
	return defaultListeningAddressList, detectedGatewayInterfaces
}

func GetScaledTimeout(base int64, duration time.Duration) time.Duration {
	return time.Duration(base*timeoutLatencyMultiplier) * duration
}

func ScaleUpTimeout() {
	timeoutLatencyMultiplier *= 2
	if timeoutLatencyMultiplier > MAX_TIMEOUT_MULTIPLER {
		timeoutLatencyMultiplier = MAX_TIMEOUT_MULTIPLER
	}

	logger.Info("Timeout multiplier set to %d", timeoutLatencyMultiplier)
}

func ResetScaleTimeout() {
	timeoutLatencyMultiplier = 1
	logger.Info("Timeout multiplier set to %d", timeoutLatencyMultiplier)
}
