package shared

import (
	"fmt"
	"github.com/jackpal/gateway"
	"net/url"
	"strings"

	"github.com/parvit/qpep/logger"
)

var (
	UsingProxy                = false
	ProxyAddress              *url.URL
	defaultListeningAddress   string
	detectedGatewayInterfaces []int64
	detectedGatewayAddresses  []string
)

func GetDefaultLanListeningAddress(currentAddress, gatewayAddress string) (string, []int64) {
	if len(defaultListeningAddress) > 0 {
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	if !strings.HasPrefix(currentAddress, "0.") && !strings.HasPrefix(currentAddress, "127.") {
		return currentAddress, detectedGatewayInterfaces
	}

	if len(gatewayAddress) == 0 {
		defaultIP, err := gateway.DiscoverInterface()
		if err != nil {
			panic(fmt.Sprintf("PANIC: Could not discover default lan address and the requested one is not suitable, error: %v\n", err))
			return currentAddress, detectedGatewayInterfaces
		}

		defaultListeningAddress = defaultIP.String()
		logger.Info("Found default ip address: %s\n", defaultListeningAddress)
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	logger.Info("WARNING: Detected invalid listening ip address, trying to autodetect the default route...\n")

	searchIdx := -1
	searchLongest := 0

NEXT:
	for i := 0; i < len(detectedGatewayAddresses); i++ {
		for idx := 0; idx < len(gatewayAddress); idx++ {
			if currentAddress[idx] == gatewayAddress[idx] {
				continue
			}
			if idx >= searchLongest {
				searchIdx = i
				searchLongest = idx
				continue NEXT
			}
		}
	}
	if searchIdx != -1 {
		defaultListeningAddress = detectedGatewayAddresses[searchIdx]
		logger.Info("Found default ip address: %s\n", defaultListeningAddress)
		return defaultListeningAddress, detectedGatewayInterfaces
	}
	defaultListeningAddress = detectedGatewayAddresses[0]
	return defaultListeningAddress, detectedGatewayInterfaces
}

func GetLanListeningAddresses() ([]string, []int64) {
	return detectedGatewayAddresses, detectedGatewayInterfaces
}
