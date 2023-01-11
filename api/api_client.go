package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	. "github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
)

func getClientForAPI(localAddr net.Addr) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				Info("API Proxy: %v %v\n", shared.UsingProxy, shared.ProxyAddress)
				if shared.UsingProxy {
					return shared.ProxyAddress, nil
				}
				return nil, nil
			},
			DialContext: (&net.Dialer{
				LocalAddr: localAddr,
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:    1,
			IdleConnTimeout: 10 * time.Second,
			//TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func RequestEcho(localAddress, address string, port int, toServer bool) *EchoResponse {
	prefix := API_PREFIX_CLIENT
	if toServer {
		prefix = API_PREFIX_SERVER
	}
	addr := fmt.Sprintf("http://%s:%d%s", address, port, prefix+API_ECHO_PATH)

	resolvedAddr, errAddr := net.ResolveTCPAddr("tcp", localAddress+":0")
	if errAddr != nil {
		Info("ERROR: %v\n", errAddr)
		return nil
	}

	clientInst := getClientForAPI(resolvedAddr)

	req, err := http.NewRequest("GET", addr, nil)
	if err != nil {
		Info("1 ERROR: %v\n", err)
		return nil
	}
	req.Header.Set("User-Agent", runtime.GOOS)

	resp, err := clientInst.Do(req)
	if err != nil {
		Info("2 ERROR: %v\n", err)
		return nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		Info("ERROR: BAD status code %d\n", resp.StatusCode)
		return nil
	}

	str := &bytes.Buffer{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		str.WriteString(scanner.Text())
	}

	if scanner.Err() != nil {
		Info("3 ERROR: %v\n", scanner.Err())
		return nil
	}

	if shared.QPepConfig.Verbose {
		Info("%s\n", str.String())
	}

	respData := &EchoResponse{}
	jsonErr := json.Unmarshal(str.Bytes(), &respData)
	if jsonErr != nil {
		Info("4 ERROR: %v\n", jsonErr)
		return nil
	}

	Statistics.SetState(INFO_OTHER_VERSION, respData.ServerVersion)
	return respData
}

func RequestStatus(localAddress, gatewayAddress string, apiPort int, publicAddress string, toServer bool) *StatusReponse {
	prefix := API_PREFIX_CLIENT
	if toServer {
		prefix = API_PREFIX_SERVER
	}
	apiPath := strings.Replace(prefix+API_STATUS_PATH, ":addr", publicAddress, -1)
	addr := fmt.Sprintf("http://%s:%d%s", gatewayAddress, apiPort, apiPath)

	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP(localAddress),
	})

	resp, err := client.Get(addr)
	if err != nil {
		Info("5 ERROR: %v\n", err)
		return nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		Info("ERROR: BAD status code %d\n", resp.StatusCode)
		return nil
	}

	str := &bytes.Buffer{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		str.WriteString(scanner.Text())
	}

	if scanner.Err() != nil {
		Info("6 ERROR: %v\n", scanner.Err())
		return nil
	}

	if shared.QPepConfig.Verbose {
		Info("%s\n", str.String())
	}

	respData := &StatusReponse{}
	jsonErr := json.Unmarshal(str.Bytes(), &respData)
	if jsonErr != nil {
		Info("7 ERROR: %v\n", jsonErr)
		return nil
	}

	return respData
}

func RequestStatistics(localAddress, gatewayAddress string, apiPort int, publicAddress string) *StatsInfoReponse {
	apiPath := strings.Replace(API_PREFIX_SERVER+API_STATS_DATA_SRV_PATH, ":addr", publicAddress, -1)
	addr := fmt.Sprintf("http://%s:%d%s", gatewayAddress, apiPort, apiPath)

	clientInst := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP(localAddress),
	})

	resp, err := clientInst.Get(addr)
	if err != nil {
		Info("8 ERROR: %v\n", err)
		return nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		Info("ERROR: BAD status code %d\n", resp.StatusCode)
		return nil
	}

	str := &bytes.Buffer{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		str.WriteString(scanner.Text())
	}

	if scanner.Err() != nil {
		Info("9 ERROR: %v\n", scanner.Err())
		return nil
	}

	if shared.QPepConfig.Verbose {
		Info("%s\n", str.String())
	}

	respData := &StatsInfoReponse{}
	jsonErr := json.Unmarshal(str.Bytes(), &respData)
	if jsonErr != nil {
		Info("10 ERROR: %v\n", jsonErr)
		return nil
	}

	return respData
}
