package shared

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
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
	errNoGateway     = errors.New("no gateway found")
	repl             = strings.NewReplacer(PROXY_KEY_1, "", PROXY_KEY_3, "", PROXY_KEY_4, "")
	userRegistryKeys = make([]string, 0, 8)
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

	// get interfaces with default routes set
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

	// parse the configuration of the interfaces to extract the addresses
	configCmd := exec.Command("netsh", "interface", "ip", "show", "config")
	configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err = configCmd.CombinedOutput()
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

	var configCmd *exec.Cmd
	if !active {
		for _, userKey := range userRegistryKeys {
			log.Printf("Clearing system proxy settings\n")
			configCmd = exec.Command("reg", "add", userKey,
				"/v", "ProxyServer", "/t", "REG_SZ", "/d",
				"", "/f")
			configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			configCmd.Run()

			configCmd = exec.Command("reg", "add", userKey,
				"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
			configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			configCmd.Run()
		}

		UsingProxy = false
		ProxyAddress = nil
		return
	}

	log.Printf("Setting system proxy to '%s:%d'\n", QuicConfiguration.ListenIP, QuicConfiguration.ListenPort)
	for _, userKey := range userRegistryKeys {
		configCmd = exec.Command("reg", "add", userKey,
			"/v", "ProxyServer", "/t", "REG_SZ", "/d",
			fmt.Sprintf("%s:%d", QuicConfiguration.ListenIP, QuicConfiguration.ListenPort), "/f")
		configCmd.Run()

		configCmd = exec.Command("reg", "add", userKey,
			"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
		configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		configCmd.Run()
	}

	Flush()

	urlValue, err := url.Parse(fmt.Sprintf("http://%s:%d", QuicConfiguration.ListenIP, QuicConfiguration.ListenPort))
	if err != nil {
		panic(err)
	}
	ProxyAddress = urlValue
	UsingProxy = true
}

func GetSystemProxyEnabled() (bool, *url.URL) {
	configCmd := exec.Command("reg", "query", PROXY_KEY_1,
		"/v", "ProxyEnable")
	configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	data, err := configCmd.CombinedOutput()
	if err != nil {
		log.Printf("ERR: %v\n", err)
		return false, nil
	}
	if strings.Index(string(data), "0x1") != -1 {
		proxyCmd := exec.Command("reg", "query", PROXY_KEY_1,
			"/v", "ProxyServer")
		proxyCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		data2, err := proxyCmd.CombinedOutput()
		if err != nil {
			log.Printf("ERR: %v\n", err)
			return false, nil
		}

		proxyUrlString := strings.TrimSpace(repl.Replace(string(data2)))
		proxyUrl, err := url.Parse("http://" + proxyUrlString)
		if err == nil {
			return true, proxyUrl
		}
	}
	return false, nil
}

func preloadRegistryKeysForUsers() {
	if len(userRegistryKeys) > 0 {
		return
	}

	configCmd := exec.Command("wmic", "useraccount", "get", "sid")
	configCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	data, err := configCmd.CombinedOutput()
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
			userRegistryKeys = append(userRegistryKeys, fmt.Sprintf(PROXY_KEY_2, key))
		}
	}
}

var (
	wininet, _           = syscall.LoadLibrary("Wininet.dll")
	internetSetOption, _ = syscall.GetProcAddress(wininet, "InternetSetOptionW")
)

const (
	_INTERNET_OPTION_PER_CONNECTION_OPTION  = 75
	_INTERNET_OPTION_PROXY_SETTINGS_CHANGED = 95
	_INTERNET_OPTION_REFRESH                = 37
)

const (
	_PROXY_TYPE_DIRECT         = 0x00000001 // direct to net
	_PROXY_TYPE_PROXY          = 0x00000002 // via named proxy
	_PROXY_TYPE_AUTO_PROXY_URL = 0x00000004 // autoproxy URL
	_PROXY_TYPE_AUTO_DETECT    = 0x00000008 // use autoproxy detection
)

const (
	_INTERNET_PER_CONN_FLAGS                        = 1
	_INTERNET_PER_CONN_PROXY_SERVER                 = 2
	_INTERNET_PER_CONN_PROXY_BYPASS                 = 3
	_INTERNET_PER_CONN_AUTOCONFIG_URL               = 4
	_INTERNET_PER_CONN_AUTODISCOVERY_FLAGS          = 5
	_INTERNET_PER_CONN_AUTOCONFIG_SECONDARY_URL     = 6
	_INTERNET_PER_CONN_AUTOCONFIG_RELOAD_DELAY_MINS = 7
	_INTERNET_PER_CONN_AUTOCONFIG_LAST_DETECT_TIME  = 8
	_INTERNET_PER_CONN_AUTOCONFIG_LAST_DETECT_URL   = 9
	_INTERNET_PER_CONN_FLAGS_UI                     = 10
)

type internetPerConnOptionList struct {
	dwSize        uint32
	pszConnection *uint16
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      uintptr
}

type internetPreConnOption struct {
	dwOption uint32
	value    uint64
}

// stringPtrAddr 获取C字符串(UTF16)的数组第一个位置的地址
func stringPtrAddr(str string) (uint64, error) {
	scriptLocPtr, err := syscall.UTF16PtrFromString(str)
	if err != nil {
		return 0, err
	}
	n := new(big.Int)
	n.SetString(fmt.Sprintf("%x\n", scriptLocPtr), 16)
	return n.Uint64(), nil
}

// newParam 创建参数序列容器
// n: 参数数量
func newParam(n int) internetPerConnOptionList {
	return internetPerConnOptionList{
		dwSize:        4,
		pszConnection: nil,
		dwOptionCount: uint32(n),
		dwOptionError: 0,
		pOptions:      0,
	}
}

// SetGlobalProxy 设置全局代理
// proxyServer: 代理服务器host:port，例如: "127.0.0.1:7890"
// bypass: 忽略代理列表,这些配置项开头的地址不进行代理
func SetGlobalProxy(proxyServer string, bypasses ...string) error {
	if proxyServer == "" {
		return errors.New("代理服务器(proxyServer)配置为空")
	}

	proxyServerPtrAddr, err := stringPtrAddr(proxyServer)
	if err != nil {
		return err
	}

	var bypassBuilder strings.Builder
	// 地址过滤配置
	if bypasses != nil {
		for _, item := range bypasses {
			bypassBuilder.WriteString(item)
			bypassBuilder.WriteByte(';')
		}
	} else {
		bypassBuilder.WriteString("<local>")
	}
	bypassAddr, err := stringPtrAddr(bypassBuilder.String())
	if err != nil {
		return err
	}

	param := newParam(3)
	options := []internetPreConnOption{
		{dwOption: _INTERNET_PER_CONN_FLAGS, value: _PROXY_TYPE_PROXY | _PROXY_TYPE_DIRECT},
		{dwOption: _INTERNET_PER_CONN_PROXY_SERVER, value: proxyServerPtrAddr},
		{dwOption: _INTERNET_PER_CONN_PROXY_BYPASS, value: bypassAddr},
	}
	param.pOptions = uintptr(unsafe.Pointer(&options[0]))
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PER_CONNECTION_OPTION,
		uintptr(unsafe.Pointer(&param)),
		unsafe.Sizeof(param),
		0, 0)
	// fmt.Printf(">> Ret [%d] Setting options: %s\n", ret, infoPtr)
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}

	return Flush()
}

// Off 关闭代理
func Off() error {
	param := newParam(1)
	option := internetPreConnOption{
		dwOption: _INTERNET_PER_CONN_FLAGS,
		//value:    _PROXY_TYPE_AUTO_DETECT | _PROXY_TYPE_DIRECT}
		value: _PROXY_TYPE_DIRECT}
	param.pOptions = uintptr(unsafe.Pointer(&option))
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PER_CONNECTION_OPTION,
		uintptr(unsafe.Pointer(&param)),
		unsafe.Sizeof(param),
		0, 0)
	// fmt.Printf(">> Ret [%d] Setting options: %s\n", ret, info)
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}
	return Flush()
}

// Flush 更新系统配置使生效
func Flush() error {
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PROXY_SETTINGS_CHANGED,
		0, 0,
		0, 0)
	// fmt.Println(">> Propagating changes:", fmt.Sprintf("%s", errno))
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}

	ret, _, infoPtr = syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_REFRESH,
		0, 0,
		0, 0)
	// fmt.Println(">> Refreshing:", errno)
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}
	return nil
}
