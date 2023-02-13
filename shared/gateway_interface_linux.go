package shared

/**
* Parts of the code similar to the github.com/jackpal/gateway module
* but the command output parse is different to allow extract also the
* interface ID for interface filtering in the divert engine
 */

import (
	"github.com/jackpal/gateway"
	"github.com/parvit/qpep/logger"
	"net/url"
)

func getRouteGatewayInterfaces() ([]int64, []string, error) {
	defaultIP, err := gateway.DiscoverInterface()
	if err != nil {
		logger.Panic("Could not discover default lan address and the requested one is not suitable, error: %v", err)
	}

	logger.Info("Found default ip address: %s\n", defaultIP.String())
	return []int64{}, []string{defaultIP.String()}, nil
}

func SetSystemProxy(active bool) {
	if !active {
		logger.Info("Clearing system proxy settings\n")
		return
	}
	logger.Info("Setting system proxy not yet supported\n")
}

func GetSystemProxyEnabled() (bool, *url.URL) {
	return false, nil
}
