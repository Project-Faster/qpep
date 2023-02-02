package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/parvit/qpep/logger"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/parvit/qpep/shared"
)

func getClientForAPI(localAddr net.Addr) *http.Client {
	dialer := &net.Dialer{
		LocalAddr: localAddr,
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				logger.Info("API Proxy: %v %v\n", shared.UsingProxy, shared.ProxyAddress)
				if shared.UsingProxy {
					return shared.ProxyAddress, nil
				}
				return nil, nil
			},
			DialContext:     dialer.DialContext,
			MaxIdleConns:    1,
			IdleConnTimeout: 10 * time.Second,
			//TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func doAPIRequest(addr string, client *http.Client) (*http.Response, error) {
	req, err := http.NewRequest("GET", addr, nil)
	if err != nil {
		logger.Error("1  %v\n", err)
		return nil, err
	}
	req.Header.Set("User-Agent", runtime.GOOS)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("2  %v\n", err)
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp, nil
}

func RequestEcho(localAddress, address string, port int, toServer bool) *EchoResponse {
	prefix := API_PREFIX_CLIENT
	if toServer {
		prefix = API_PREFIX_SERVER
	}
	addr := fmt.Sprintf("http://%s:%d%s", address, port, prefix+API_ECHO_PATH)

	resolvedAddr, errAddr := net.ResolveTCPAddr("tcp", localAddress+":0")
	if errAddr != nil {
		logger.Error(" %v\n", errAddr)
		return nil
	}

	clientInst := getClientForAPI(resolvedAddr)

	resp, err := doAPIRequest(addr, clientInst)
	if err != nil {
		logger.Error("2  %v\n", err)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error(" BAD status code %d\n", resp.StatusCode)
		return nil
	}

	str := &bytes.Buffer{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		str.WriteString(scanner.Text())
	}

	if scanner.Err() != nil {
		logger.Error("3  %v\n", scanner.Err())
		return nil
	}

	if shared.QPepConfig.Verbose {
		logger.Error("%s\n", str.String())
	}

	respData := &EchoResponse{}
	jsonErr := json.Unmarshal(str.Bytes(), &respData)
	if jsonErr != nil {
		logger.Error("4  %v\n", jsonErr)
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

	clientInst := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP(localAddress),
	})

	resp, err := doAPIRequest(addr, clientInst)
	if err != nil {
		logger.Error("2  %v\n", err)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error(" BAD status code %d\n", resp.StatusCode)
		return nil
	}

	str := &bytes.Buffer{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		str.WriteString(scanner.Text())
	}

	if scanner.Err() != nil {
		logger.Error("6  %v\n", scanner.Err())
		return nil
	}

	if shared.QPepConfig.Verbose {
		logger.Error("%s\n", str.String())
	}

	respData := &StatusReponse{}
	jsonErr := json.Unmarshal(str.Bytes(), &respData)
	if jsonErr != nil {
		logger.Error("7  %v\n", jsonErr)
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

	resp, err := doAPIRequest(addr, clientInst)
	if err != nil {
		logger.Error("2  %v\n", err)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error(" BAD status code %d\n", resp.StatusCode)
		return nil
	}

	str := &bytes.Buffer{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		str.WriteString(scanner.Text())
	}

	if scanner.Err() != nil {
		logger.Error("9  %v\n", scanner.Err())
		return nil
	}

	if shared.QPepConfig.Verbose {
		logger.Error("%s\n", str.String())
	}

	respData := &StatsInfoReponse{}
	jsonErr := json.Unmarshal(str.Bytes(), &respData)
	if jsonErr != nil {
		logger.Error("10  %v\n", jsonErr)
		return nil
	}

	return respData
}
