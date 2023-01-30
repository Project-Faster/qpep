package shared

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net/url"
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

const (
	PROXY_KEY_1 = `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	PROXY_KEY_2 = `HKEY_USERS\%s\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	PROXY_KEY_3 = `ProxyServer`
	PROXY_KEY_4 = `REG_SZ`
)

var (
	errNoGateway      = errors.New("no gateway found")
	repl              = strings.NewReplacer(PROXY_KEY_1, "", PROXY_KEY_3, "", PROXY_KEY_4, "")
	usersRegistryKeys = make([]string, 0, 8)
)

func runCommand(name string, cmd ...string) ([]byte, error) {
	routeCmd := exec.Command(name, cmd...)
	routeCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return routeCmd.CombinedOutput()
}

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

	// get interfaces with default routes set
	output, err := runCommand("netsh", "interface", "ip", "show", "route")
	if err != nil {
		return nil, nil, err
	}

	var routeInterfaceMap = make(map[string]int64)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
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
	output, err = runCommand("netsh", "interface", "ip", "show", "interface")
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

	// parse the configuration of the interfaces to extract the addresses
	output, err = runCommand("netsh", "interface", "ip", "show", "config")
	if err != nil {
		return nil, nil, err
	}

	rx := regexp.MustCompile(`.+"([^"]+)"`)

	scn := bufio.NewScanner(strings.NewReader(string(output)))
	scn.Split(bufio.ScanLines)

	var interfacesList = make([]int64, 0, len(routeInterfaceMap))
	var addressesList = make([]string, 0, len(routeInterfaceMap))

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

		for scn.Scan() {
			line = strings.TrimSpace(scn.Text())
			if len(line) == 0 {
				continue BLOCK
			}

			idx := strings.LastIndex(line, "IP")
			if idx != -1 {
				pieces := strings.Split(line, ":")
				if len(pieces) != 2 {
					continue
				}
				line = strings.TrimSpace(pieces[1])
				if strings.HasPrefix(line, "127.") {
					continue
				}
				addressesList = append(addressesList, line)
				interfacesList = append(interfacesList, value)
				continue BLOCK
			}
		}
	}
	return interfacesList, addressesList, nil
}

func SetSystemProxy(active bool) {
	preloadRegistryKeysForUsers()

	if !active {
		for _, userKey := range usersRegistryKeys {
			log.Printf("Clearing system proxy settings\n")
			_, _ = runCommand("reg", "add", userKey,
				"/v", "ProxyServer", "/t", "REG_SZ", "/d",
				"", "/f")

			_, _ = runCommand("reg", "add", userKey,
				"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
		}

		UsingProxy = false
		ProxyAddress = nil
		return
	}

	log.Printf("Setting system proxy to '%s:%d'\n", QPepConfig.ListenHost, QPepConfig.ListenPort)
	for _, userKey := range usersRegistryKeys {
		_, _ = runCommand("reg", "add", userKey,
			"/v", "ProxyServer", "/t", "REG_SZ", "/d",
			fmt.Sprintf("%s:%d", QPepConfig.ListenHost, QPepConfig.ListenPort), "/f")

		_, _ = runCommand("reg", "add", userKey,
			"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
	}

	Flush()

	urlValue, err := url.Parse(fmt.Sprintf("http://%s:%d", QPepConfig.ListenHost, QPepConfig.ListenPort))
	if err != nil {
		panic(err)
	}
	ProxyAddress = urlValue
	UsingProxy = true
}

func GetSystemProxyEnabled() (bool, *url.URL) {
	data, err := runCommand("reg", "query", PROXY_KEY_1,
		"/v", "ProxyEnable")
	if err != nil {
		log.Printf("ERR: %v\n", err)
		return false, nil
	}
	if strings.Index(string(data), "0x1") != -1 {
		data, err = runCommand("reg", "query", PROXY_KEY_1,
			"/v", "ProxyServer")
		if err != nil {
			log.Printf("ERR: %v\n", err)
			return false, nil
		}

		proxyUrlString := strings.TrimSpace(repl.Replace(string(data)))
		proxyUrl, err := url.Parse("http://" + proxyUrlString)
		if err == nil {
			return true, proxyUrl
		}
	}
	return false, nil
}

func preloadRegistryKeysForUsers() {
	if len(usersRegistryKeys) > 0 {
		return
	}

	data, err := runCommand("wmic", "useraccount", "get", "sid")
	if err != nil {
		log.Printf("ERR: %v\n", err)
		panic(fmt.Sprintf("ERR: %v", err))
	}

	scn := bufio.NewScanner(strings.NewReader(string(data)))
	scn.Split(bufio.ScanLines)

	for scn.Scan() {
		line := scn.Text()
		if strings.HasPrefix(line, "SID") {
			continue
		}

		key := strings.TrimSpace(line)
		if len(key) > 0 {
			usersRegistryKeys = append(usersRegistryKeys, fmt.Sprintf(PROXY_KEY_2, key))
		}
	}
}

// Adapted from github.com/Trisia/gosysproxy
var (
	wininet, _           = syscall.LoadLibrary("Wininet.dll")
	internetSetOption, _ = syscall.GetProcAddress(wininet, "InternetSetOptionW")
)

const (
	_INTERNET_OPTION_PROXY_SETTINGS_CHANGED = 95
	_INTERNET_OPTION_REFRESH                = 37
)

// Flush proxy configuration
func Flush() {
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PROXY_SETTINGS_CHANGED,
		0, 0,
		0, 0)
	if ret != 1 {
		log.Printf("Error propagating proxy setting: %s\n", infoPtr)
	}

	ret, _, infoPtr = syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_REFRESH,
		0, 0,
		0, 0)
	if ret != 1 {
		log.Printf("Error refreshing proxy setting: %s\n", infoPtr)
	}
}

// Adapted from github.com/Trisia/gosysproxy
