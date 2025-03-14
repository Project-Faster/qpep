package gateway

import (
	"bufio"
	"fmt"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/windivert"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// PROXY_KEY_1 Registy key path related to the current user for proxy settings
	PROXY_KEY_1 = `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	// PROXY_KEY_2 Registy key path related to a specific user for proxy settings
	PROXY_KEY_2 = `HKEY_USERS\%s\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	// PROXY_KEY_ENABLE Registry key for enabling the system proxy
	PROXY_KEY_ENABLE = `ProxyEnable`
	// PROXY_KEY_HOST Registry key for indicating the address of the system proxy
	PROXY_KEY_HOST = `ProxyServer`
	// PROXY_TYPE_SZ Registry key value type for string values
	PROXY_TYPE_SZ = `REG_SZ`
	// PROXY_TYPE_DWORD Registry key value type for integer values
	PROXY_TYPE_DWORD = `REG_DWORD`
)

var (
	// proxyAddrReplacer cleans the value from registry command to extract the current proxy address
	proxyAddrReplacer = strings.NewReplacer(PROXY_KEY_1, "", PROXY_KEY_HOST, "", PROXY_TYPE_SZ, "")
	// usersRegistryKeys array of precomputed strings for every user's proxy settings key path
	usersRegistryKeys = make([]string, 0, 8)
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

func getRouteListeningAddresses() []string {
	if defaultListeningAddress == "" {
		defaultListeningAddress = detectedGatewayAddresses[0]
	}
	return append([]string{}, detectedGatewayAddresses...)
}

// getRouteGatewayInterfaces method extracts routing information using the "netsh" utility and returns specifically:
// * Network Interface IDs list of all network interfaces configured
// * Network Address List for every configured interface
// * error in
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
	output, err, _ := shared.RunCommand("netsh", "interface", "ip", "show", "route")
	if err != nil {
		logger.Error("ERR: %v", err)
		return nil, nil, errors.ErrFailedGatewayDetect
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
		logger.Error("ERR: %v", err)
		return nil, nil, errors.ErrFailedGatewayDetect
	}

	// get the associated names of the interfaces
	output, err, _ = shared.RunCommand("netsh", "interface", "ip", "show", "interface")
	if err != nil {
		return nil, nil, errors.ErrFailedGatewayDetect
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
	output, err, _ = shared.RunCommand("netsh", "interface", "ip", "show", "config")
	if err != nil {
		logger.Error("ERR: %v", err)
		return nil, nil, errors.ErrFailedGatewayDetect
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
	if active && ProxyAddress != nil {
		return
	}

	preloadRegistryKeysForUsers()

	if !active {
		if !shared.DEBUG_MASK_REDIRECT {
			for _, userKey := range usersRegistryKeys {
				logger.Info("Clearing system proxy settings\n")
				_, _, _ = shared.RunCommand("reg", "add", userKey,
					"/v", PROXY_KEY_HOST, "/t", PROXY_TYPE_SZ, "/d",
					"", "/f")

				_, _, _ = shared.RunCommand("reg", "add", userKey,
					"/v", PROXY_KEY_ENABLE, "/t", PROXY_TYPE_DWORD, "/d", "0", "/f")
			}
		}

		UsingProxy = false
		ProxyAddress = nil
		return
	}

	config := configuration.QPepConfig.Client
	logger.Info("Setting system proxy to '%s:%d'\n", config.LocalListeningAddress, config.LocalListenPort)
	if !shared.DEBUG_MASK_REDIRECT {
		for _, userKey := range usersRegistryKeys {
			_, _, _ = shared.RunCommand("reg", "add", userKey,
				"/v", PROXY_KEY_HOST, "/t", PROXY_TYPE_SZ, "/d",
				fmt.Sprintf("%s:%d", config.LocalListeningAddress, config.LocalListenPort), "/f")

			_, _, _ = shared.RunCommand("reg", "add", userKey,
				"/v", PROXY_KEY_ENABLE, "/t", PROXY_TYPE_DWORD, "/d",
				"1", "/f")
		}

		Flush()
	}

	urlValue, err := url.Parse(fmt.Sprintf("http://%s:%d", config.LocalListeningAddress, config.LocalListenPort))
	if err != nil {
		panic(err)
	}
	ProxyAddress = urlValue
	UsingProxy = true
}

func GetSystemProxyEnabled() (bool, *url.URL) {
	data, err, _ := shared.RunCommand("reg", "query", PROXY_KEY_1,
		"/v", PROXY_KEY_ENABLE)
	if err != nil {
		logger.Info("ERR: %v\n", err)
		return false, nil
	}
	if strings.Index(string(data), "0x1") != -1 {
		data, err, _ = shared.RunCommand("reg", "query", PROXY_KEY_1,
			"/v", PROXY_KEY_HOST)
		if err != nil {
			logger.Info("ERR: %v\n", err)
			return false, nil
		}

		proxyUrlString := strings.TrimSpace(proxyAddrReplacer.Replace(string(data)))
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

	data, err, _ := shared.RunCommand("wmic", "useraccount", "get", "sid")
	if err != nil {
		logger.Info("ERR: %v\n", err)
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

func SetConnectionDiverter(active bool, gatewayAddr, listenAddr string,
	gatewayPort, listenPort, numThreads int,
	gatewayInterface int64, ignoredPorts []int) bool {

	if active {
		logger.Info("Initializing WinDivert: %v %v %v %v %v %v %v\n",
			gatewayAddr, listenAddr, gatewayPort, listenPort, numThreads, gatewayInterface, ignoredPorts)

		code := windivert.InitializeWinDivertEngine(gatewayAddr, listenAddr, gatewayPort,
			listenPort, numThreads, gatewayInterface, ignoredPorts)
		logger.Info("WinDivert code: %v\n", code)
		if code != windivert.DIVERT_OK {
			logger.Error("Could not initialize WinDivert engine, code %d\n", code)
		}
		return code == windivert.DIVERT_OK
	}

	logger.Info("Closing WinDivert...\n")
	code := windivert.CloseWinDivertEngine()
	logger.Info("WinDivert code: %v\n", code)
	return code == windivert.DIVERT_OK
}

func GetConnectionDivertedState(local, remote *net.TCPAddr) (bool, int, int, string, string) {
	if remote == nil {
		return false, -1, -1, "", ""
	}
	result, srcPort, dstPort, srcAddress, dstAddress := windivert.GetConnectionStateData(remote.Port)
	return result == windivert.DIVERT_OK, srcPort, dstPort, srcAddress, dstAddress
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
		logger.Info("Error propagating proxy setting: %s\n", infoPtr)
	}

	ret, _, infoPtr = syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_REFRESH,
		0, 0,
		0, 0)
	if ret != 1 {
		logger.Info("Error refreshing proxy setting: %s\n", infoPtr)
	}
}

// Adapted from github.com/Trisia/gosysproxy
