package shared

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

/**
* Parts of the code similar to the github.com/jackpal/gateway module
* but the command output parse is different to allow extract also the
* interface ID for interface filtering in the divert engine
 */

var (
	errNoGateway = errors.New("no gateway found")
)

func getRouteGatewayInterfaces() ([]int64, error) {
	// Windows route output format is always like this:
	// Tipo pubblicazione      Prefisso met.                  Gateway idx/Nome interfaccia
	// -------  --------  ---  ------------------------  ---  ------------------------
	// No       Manuale   0    0.0.0.0/0                  18  192.168.1.1
	// No       Manuale   0    0.0.0.0/0                  20  192.168.1.1
	// No       Sistema   256  127.0.0.0/8                 1  Loopback Pseudo-Interface 1
	// No       Sistema   256  127.0.0.1/32                1  Loopback Pseudo-Interface 1
	// No       Sistema   256  127.255.255.255/32          1  Loopback Pseudo-Interface 1
	// No       Sistema   256  192.168.1.0/24             18  Wi-Fi
	// No       Sistema   256  192.168.1.0/24             20  Ethernet
	// No       Sistema   256  192.168.1.5/32             20  Ethernet
	// No       Sistema   256  192.168.1.30/32            18  Wi-Fi
	// No       Sistema   256  192.168.1.255/32           18  Wi-Fi

	routeCmd := exec.Command("netsh", "interface", "ip", "show", "route")
	routeCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var interfacesList []int64 = make([]int64, 0, 8)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 6 {
			continue
		}

		if fields[3] != "0.0.0.0/0" {
			continue
		}

		value, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			continue
		}
		interfacesList = append(interfacesList, value)
	}
	if len(interfacesList) == 0 {
		return nil, errNoGateway
	}
	return interfacesList, nil
}
