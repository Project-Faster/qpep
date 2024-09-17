package gateway

import (
	"fmt"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/jackpal/gateway"
	"net"
	"net/url"
)

const (
	// 25       smtp
	// 21       ftp
	// 22       ssh
	// 23       telnet
	// 53       dns
	// 67, 68.  dhcp
	// 80, 443  http/https
	// 143      imap
	// 161, 162 snmp
	// 636      ldap
	// 989, 990 sftp
	// 1025:65535  other tcp
	PROTOCOLS_PORTS_LIST = `21:25,53,67,68,80,443,143,636,161:162,989:990,1025:65535`
)

var redirectOn = false

func getRouteListeningAddresses() []string {
	if defaultListeningAddress == "" {
		defaultListeningAddress = "127.0.0.1"
	}
	return []string{defaultListeningAddress}
}

func getRouteGatewayInterfaces() ([]int64, []string, error) {
	defaultIP, err := gateway.DiscoverInterface()
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

func SetConnectionDiverter(active bool, gatewayAddr, listenAddr string, gatewayPort, listenPort, numThreads int, gatewayInterface int64) bool {
	redirectOn = active

	if active {
		logger.Info("Setting system iptables\n")

		_, err, _ := shared.RunCommand("bash", "-c", "iptables -P FORWARD ACCEPT")
		if err != nil {
			logger.Error("%v\n", err)
			return false
		}

		_, err, _ = shared.RunCommand("bash", "-c",
			fmt.Sprintf("iptables -t nat -A OUTPUT -j DNAT -p tcp --to-destination %s:%d -m multiport --destination-ports %s", listenAddr, listenPort, PROTOCOLS_PORTS_LIST))
		if err != nil {
			logger.Error("%v\n", err)
			return false
		}

		_, err, _ = shared.RunCommand("bash", "-c",
			fmt.Sprintf("iptables -t nat -A POSTROUTING -j MASQUERADE -p tcp -m multiport --destination-ports %s", PROTOCOLS_PORTS_LIST))
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
