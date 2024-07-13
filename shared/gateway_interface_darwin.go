package shared

/**
* Parts of the code similar to the github.com/jackpal/gateway module
* but the command output parse is different to allow extract also the
* interface ID for interface filtering in the divert engine
 */

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/Project-Faster/qpep/logger"
	"github.com/jackpal/gateway"
)

// notes
// networksetup -listallnetworkservices -> All network interfaces available
// networksetup -setwebproxy "Wi-fi" 127.0.0.1 8080 -> HTTP proxy
// networksetup -setwebproxystate "Wi-fi" on -> activate HTTP proxy
// networksetup -setsecurewebproxy "Wi-fi" 127.0.0.1 8443 -> HTTPS proxy
// networksetup -setsecurewebproxystate "Wi-fi" on -> activate HTTPS proxy

var (
	enabledSep = []byte(`Enabled: Yes`)
	serverSep  = []byte(`Server: `)
	portSep    = []byte(`Port: `)
	ipSep      = []byte(`IP address: `)
	newLineSep = []byte("\n")

	ipRegexp = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
)

// RunCommand method abstracts the execution of a system command and returns the combined stdout,stderr streams and
// an error if there was any issue with the command executed
func RunCommand(name string, cmd ...string) ([]byte, error, int) {
	routeCmd := exec.Command(name, cmd...)
	routeCmd.SysProcAttr = &syscall.SysProcAttr{}
	result, err := routeCmd.CombinedOutput()
	code := routeCmd.ProcessState.ExitCode()

	return result, err, code
}

func getRouteGatewayInterfaces() ([]int64, []string, error) {
	defaultIP, err := gateway.DiscoverGateway()
	if err != nil {
		logger.Panic("Could not discover default lan address and the requested one is not suitable, error: %v", err)
	}

	logger.Info("Found default ip address: %s\n", defaultIP.String())
	return []int64{}, []string{defaultIP.String()}, nil
}

func getRouteListeningAddresses() []string {
	output, err, code := RunCommand("networksetup", "-listallnetworkservices")
	if err != nil || code != 0 {
		logger.Error("Could not set system proxy, error (code: %d): %v", code, err)
		return []string{}
	}

	outAddress := []string{}

	scn := bufio.NewScanner(bytes.NewReader(output))
	scn.Split(bufio.ScanLines)

	for scn.Scan() {
		iface := strings.TrimSpace(scn.Text())
		if len(iface) == 0 || strings.Contains(iface, "*") {
			continue
		}

		// http proxy values
		output, _, _ := RunCommand("networksetup", "-getinfo", iface)

		if idx := bytes.Index(output, ipSep); idx != 0 {
			start := idx + len(ipSep)
			end := bytes.Index(output[idx:], []byte("\n")) + idx

			if ipRegexp.Match(output[start:end]) {
				outAddress = append(outAddress, string(output[start:end]))
			}
		}
	}

	return outAddress
}

func SetSystemProxy(active bool) {
	if !active {
		setAllInterfacesToProxy(QPepConfig.ListenHost, int64(QPepConfig.ListenPort), false)

		logger.Info("Clearing system proxy settings\n")
		ProxyAddress = nil
		UsingProxy = false
		return
	}

	setAllInterfacesToProxy(QPepConfig.ListenHost, int64(QPepConfig.ListenPort), true)

	urlValue, err := url.Parse(fmt.Sprintf("http://%s:%d", QPepConfig.ListenHost, QPepConfig.ListenPort))
	if err != nil {
		panic(err)
	}
	ProxyAddress = urlValue
	UsingProxy = true
}

func setAllInterfacesToProxy(address string, port int64, active bool) {
	output, err, code := RunCommand("networksetup", "-listallnetworkservices")
	if err != nil || code != 0 {
		logger.Error("Could not set system proxy, error (code: %d): %v", code, err)
		return
	}

	scn := bufio.NewScanner(bytes.NewReader(output))
	scn.Split(bufio.ScanLines)

	strPort := strconv.FormatInt(port, 10)

	state := "on"
	if !active {
		address = ""
		strPort = "0"
		state = "off"
	}

	for scn.Scan() {
		iface := strings.TrimSpace(scn.Text())
		if len(iface) == 0 {
			continue
		}

		// http proxy values
		_, _, _ = RunCommand("networksetup", "-setwebproxy", iface, address, strPort)

		// https proxy values
		_, _, _ = RunCommand("networksetup", "-setsecurewebproxy", iface, address, strPort)

		// proxy enabled
		_, _, _ = RunCommand("networksetup", "-setwebproxystate", iface, state)

		_, _, _ = RunCommand("networksetup", "-setsecurewebproxystate", iface, state)

	}
}

func GetSystemProxyEnabled() (bool, *url.URL) {
	output, err, code := RunCommand("networksetup", "-listallnetworkservices")
	if err != nil || code != 0 {
		logger.Error("Could not get system proxy, error (code: %d): %v", code, err)
		return false, nil
	}

	scn := bufio.NewScanner(bytes.NewReader(output))
	scn.Split(bufio.ScanLines)

	proxyUrlString := ""

	for scn.Scan() {
		iface := strings.TrimSpace(scn.Text())
		if len(iface) == 0 || strings.Contains(iface, "*") {
			continue
		}

		// proxy enabled
		output, _, _ := RunCommand("networksetup", "-getwebproxy", iface)
		if !bytes.Contains(output, enabledSep) {
			return false, nil
		}
		httpProxy := parseProxyUrlFromOutput(output)

		output, _, _ = RunCommand("networksetup", "-getsecurewebproxy", iface)
		if !bytes.Contains(output, enabledSep) {
			return false, nil
		}
		httpsProxy := parseProxyUrlFromOutput(output)

		if len(httpProxy) == 0 || len(httpsProxy) == 0 || !strings.EqualFold(httpsProxy, httpProxy) {
			logger.Error("Could not get system proxy, http and https proxy servers for '%s' are different: %v != %v",
				iface, httpProxy, httpsProxy)
			return false, nil
		}

		proxyUrlString = httpProxy
		break
	}

	proxyUrl, err := url.Parse("http://" + proxyUrlString)
	if err == nil {
		return true, proxyUrl
	}
	return false, nil
}

func parseProxyUrlFromOutput(output []byte) string {
	if len(output) == 0 {
		logger.Error("Empty proxy output")
		return ""
	}

	proxyHost := ""
	proxyPort := ""

	fields := bytes.Split(output, newLineSep)
	for _, field := range fields {
		idx := bytes.IndexByte(field, byte(':'))
		if idx == -1 {
			continue
		}
		if bytes.HasPrefix(field, serverSep) {
			proxyHost = string(field[8:])
			continue
		}
		if bytes.HasPrefix(field, portSep) {
			proxyPort = string(field[6:])
			continue
		}
	}
	if len(proxyHost) == 0 || len(proxyPort) == 0 {
		logger.Error("Could not parse proxy output: %v", string(output))
		return ""
	}

	return fmt.Sprintf("%s:%s", proxyHost, proxyPort)
}
