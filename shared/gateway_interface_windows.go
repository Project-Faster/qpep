package shared

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

/**
* Parts of the code similar to the github.com/jackpal/gateway module
* but the command output parse is different to allow extract also the
* interface ID for interface filtering in the divert engine and the
* local ip addresses to use as source addresses
 */

var (
	errNoGateway = errors.New("no gateway found")
)

func getRouteGatewayInterfaces() ([]int64, []string, error) {
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
		return nil, nil, err
	}

	var routeInterfaceMap = make(map[string]int64)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 6 {
			continue
		}
		if !strings.Contains(fields[3], "0.0.0.0") {
			continue
		}
		value, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			continue
		}

		routeInterfaceMap[fields[4]] = value
	}
	if len(routeInterfaceMap) == 0 {
		return nil, nil, errNoGateway
	}

	// get the associated names of the interfaces
	interfaceCmd := exec.Command("netsh", "interface", "ip", "show", "interface")
	interfaceCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err = interfaceCmd.CombinedOutput()
	if err != nil {
		return nil, nil, err
	}

	lines = strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		key := strings.TrimSpace(fields[0])

		value, ok := routeInterfaceMap[key]
		if !ok {
			continue
		}

		delete(routeInterfaceMap, key)
		routeInterfaceMap[strings.Join(fields[4:], " ")] = value
	}

	configCmd := exec.Command("netsh", "interface", "ip", "show", "config")
	configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err = configCmd.CombinedOutput()
	if err != nil {
		return nil, nil, err
	}

	rx := regexp.MustCompile(`.+"([^"]+)"`)

	scn := bufio.NewScanner(strings.NewReader(string(output)))
	scn.Split(bufio.ScanLines)

	var interfacesList []int64 = make([]int64, 0, len(routeInterfaceMap))
	var addressesList []string = make([]string, 0, len(routeInterfaceMap))

BLOCK:
	for scn.Scan() {
		line := scn.Text()
		matches := rx.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}

		value, ok := routeInterfaceMap[matches[1]]
		if !ok {
			continue
		}
		interfacesList = append(interfacesList, value)

		for scn.Scan() {
			line = strings.TrimSpace(scn.Text())
			if len(line) == 0 {
				continue BLOCK
			}

			idx := strings.LastIndex(line, "IP:")
			if idx != -1 {
				line = strings.TrimSpace(line[idx+3:])
				addressesList = append(addressesList, line)
				continue BLOCK
			}
		}
	}
	return interfacesList, addressesList, nil
}

func SetSystemProxy(active bool) {
	var configCmd *exec.Cmd
	if !active {
		log.Printf("Clearing system proxy settings\n")
		configCmd = exec.Command("netsh", "winhttp", "reset", "proxy")
		configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		log.Printf("CMD: %v", configCmd.Run())

		configCmd = exec.Command("reg", "add", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
			"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
		configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		log.Printf("CMD: %v", configCmd.Run())
		return
	}

	log.Printf("Setting system proxy to '%s:%d'\n", QuicConfiguration.ListenIP, QuicConfiguration.ListenPort)
	configCmd = exec.Command("netsh", "winhttp", "set", "proxy",
		fmt.Sprintf("proxy-server=\"%s:%d\"", QuicConfiguration.ListenIP, QuicConfiguration.ListenPort ),
		"bypass-list=\"localhost\"" )
	log.Printf("CMD: %v", configCmd.Run())

	configCmd = exec.Command("reg", "add", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
		"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
	configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	log.Printf("CMD: %v", configCmd.Run())
}
