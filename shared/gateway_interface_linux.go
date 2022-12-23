package shared

/**
* Parts of the code similar to the github.com/jackpal/gateway module
* but the command output parse is different to allow extract also the
* interface ID for interface filtering in the divert engine
 */

import (
	"log"
	"net/url"
)

func getRouteGatewayInterfaces() ([]int64, []string, error) {
	return nil, nil, nil
}

func SetSystemProxy(active bool) {
	if !active {
		log.Printf("Clearing system proxy settings\n")
		return
	}
	log.Printf("Setting system proxy not yet supported\n")
}

func GetSystemProxyEnabled() (bool, *url.URL) {
	return false, ""
}
