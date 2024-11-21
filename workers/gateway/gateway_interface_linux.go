package gateway

import (
	"fmt"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/shared/logger"
	gw "github.com/jackpal/gateway"
	"net"
	"net/url"
	"strconv"
)

var (
	redirectOn = false

	defaultPortsIgnored = []int{53}
)

func getRouteListeningAddresses() []string {
	if defaultListeningAddress == "" {
		defaultListeningAddress = detectedGatewayAddresses[0]
	}
	return append([]string{}, detectedGatewayAddresses...)
}

func getRouteGatewayInterfaces() ([]int64, []string, error) {
	defaultIP, err := gw.DiscoverInterface()
	if err != nil {
		logger.Panic("Could not discover default lan address and the requested one is not suitable, error: %v", err)
	}

	logger.Info("Found default ip address: %s\n", defaultIP.String())
	return []int64{}, []string{defaultIP.String()}, nil
}

func SetSystemProxy(active bool) {}

func GetSystemProxyEnabled() (bool, *url.URL) {
	return false, nil
}

func SetConnectionDiverter(active bool, gatewayAddr, listenAddr string, gatewayPort, listenPort, numThreads int,
	gatewayInterface int64, ignoredPorts []int) bool {
	redirectOn = active

	if active {
		logger.Info("Initializing iptables: %v %v %v %v %v %v %v\n",
			gatewayAddr, listenAddr, gatewayPort, listenPort, numThreads, gatewayInterface)

		_, err, _ := shared.RunCommand("bash", "-c", "iptables -P FORWARD ACCEPT")
		if err != nil {
			logger.Error("%v\n", err)
			return false
		}

		var allIgnoredPorts = append([]int{}, defaultPortsIgnored...)
		allIgnoredPorts = append(allIgnoredPorts, gatewayPort)
		allIgnoredPorts = append(allIgnoredPorts, listenPort)
		allIgnoredPorts = append(allIgnoredPorts, ignoredPorts...)

		var allIgnoredStr = ""
		for i := 0; i < len(allIgnoredPorts); i++ {
			if i == 0 {
				allIgnoredStr = strconv.FormatInt(int64(allIgnoredPorts[i]), 10)
				continue
			}

			allIgnoredStr += "," + strconv.FormatInt(int64(allIgnoredPorts[i]), 10)
		}

		_, err, _ = shared.RunCommand("bash", "-c",
			fmt.Sprintf("iptables -t nat -A OUTPUT -j DNAT -p tcp --to-destination %s:%d -m multiport ! --dports %s", listenAddr, listenPort, allIgnoredStr))
		if err != nil {
			logger.Error("%v\n", err)
			return false
		}

		_, err, _ = shared.RunCommand("bash", "-c",
			fmt.Sprintf("iptables -t nat -A POSTROUTING -j MASQUERADE -p tcp -m multiport ! --dports %s", allIgnoredStr))
		if err != nil {
			logger.Error("%v\n", err)
			return false
		}
		return true
	}

	logger.Info("Clearing system iptables settings\n")
	_, err, _ := shared.RunCommand("bash", "-c", "iptables -F FORWARD")
	if err != nil {
		logger.Error("%v\n", err)
		return false
	}

	_, err, _ = shared.RunCommand("bash", "-c", "iptables -t nat -F OUTPUT")
	if err != nil {
		logger.Error("%v\n", err)
		return false
	}

	_, err, _ = shared.RunCommand("bash", "-c", "iptables -t nat -F POSTROUTING")
	if err != nil {
		logger.Error("%v\n", err)
		return false
	}
	return true
}

func GetConnectionDivertedState(local, remote *net.TCPAddr) (bool, int, int, string, string) {
	if local == nil || remote == nil {
		return false, -1, -1, "", ""
	}
	return redirectOn, local.Port, remote.Port, local.IP.String(), remote.IP.String()
}
