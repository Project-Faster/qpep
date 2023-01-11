package shared

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/jackpal/gateway"
	"gopkg.in/yaml.v3"

	. "github.com/parvit/qpep/logger"
)

const (
	CONFIGFILENAME = "qpep.yml"
	CONFIGPATH     = "config"
	WEBGUIURL      = "http://127.0.0.1:%d/index?mode=%s&port=%d"
	DEFAULTCONFIG  = `
acks: 10
ackdelay: 25
congestion: 4
decimate: 4
decimatetime: 100
maxretries: 10
gateway: 198.18.0.254
port: 443
apiport: 444
listenaddress: 0.0.0.0
listenport: 9443
multistream: true
verbose: false
preferproxy: false
varackdelay: 0
threads: 4
`
)

type QPepConfigType struct {
	Acks                 int    `yaml:"acks"`
	AckDelay             int    `yaml:"ackdelay"`
	Congestion           int    `yaml:"congestion"`
	MaxConnectionRetries int    `yaml:"maxretries"`
	Decimate             int    `yaml:"decimate"`
	DelayDecimate        int    `yaml:"decimatetime"`
	GatewayHost          string `yaml:"gateway"`
	GatewayPort          int    `yaml:"port"`
	GatewayAPIPort       int    `yaml:"apiport"`
	ListenHost           string `yaml:"listenaddress"`
	ListenPort           int    `yaml:"listenport"`
	MultiStream          bool   `yaml:"multistream"`
	PreferProxy          bool   `yaml:"preferproxy"`
	Verbose              bool   `yaml:"verbose"`
	VarAckDelay          int    `yaml:"varackdelay"`
	WinDivertThreads     int    `yaml:"threads"`
}

const (
	DEFAULT_REDIRECT_RETRIES = 15
)

var (
	QPepConfig                QPepConfigType
	UsingProxy                = false
	ProxyAddress              *url.URL
	defaultListeningAddress   string
	detectedGatewayInterfaces []int64
	detectedGatewayAddresses  []string
)

type QLogWriter struct {
	*bufio.Writer
}

func (mwc *QLogWriter) Close() error {
	// Noop
	return mwc.Writer.Flush()
}

func init() {
	var err error
	detectedGatewayInterfaces, detectedGatewayAddresses, err = getRouteGatewayInterfaces()

	Info("gateway interfaces: %v\n", detectedGatewayInterfaces)

	if err != nil {
		panic(err)
	}
}

func GetConfigurationPath() string {
	basedir, err := os.Executable()
	if err != nil {
		Error("Could not find executable: %s", err)
	}

	return filepath.Join(filepath.Dir(basedir), CONFIGPATH)
}

func ReadConfiguration() (outerr error) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: ", err)
			debug.PrintStack()
			outerr = errors.New(fmt.Sprintf("%v", err))
		}
	}()

	confdir := GetConfigurationPath()

	if _, err := os.Stat(confdir); errors.Is(err, os.ErrNotExist) {
		os.Mkdir(confdir, 0664)
	}

	confFile := filepath.Join(confdir, CONFIGFILENAME)
	if _, err := os.Stat(confFile); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(confFile, []byte(DEFAULTCONFIG), 0664)
	}

	f, err := os.Open(confFile)
	if err != nil {
		Error("Could not read expected configuration file: %v", err)
		return err
	}
	defer func() {
		f.Close()
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		Error("Could not read expected configuration file: %v", err)
		return err
	}
	if err := yaml.Unmarshal(data, &QPepConfig); err != nil {
		Error("Could not decode configuration file: %v", err)
		return err
	}

	Info("Configuration Loaded")
	return nil
}
func GetDefaultLanListeningAddress(currentAddress, gatewayAddress string) (string, []int64) {
	if len(defaultListeningAddress) > 0 {
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	if !strings.HasPrefix(currentAddress, "0.") && !strings.HasPrefix(currentAddress, "127.") {
		return currentAddress, detectedGatewayInterfaces
	}

	if len(gatewayAddress) == 0 {
		defaultIP, err := gateway.DiscoverInterface()
		if err != nil {
			panic(fmt.Sprintf("PANIC: Could not discover default lan address and the requested one is not suitable, error: %v\n", err))
			return currentAddress, detectedGatewayInterfaces
		}

		defaultListeningAddress = defaultIP.String()
		Info("Found default ip address: %s\n", defaultListeningAddress)
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	Info("WARNING: Detected invalid listening ip address, trying to autodetect the default route...\n")

	searchIdx := -1
	searchLongest := 0

NEXT:
	for i := 0; i < len(detectedGatewayAddresses); i++ {
		for idx := 0; idx < len(gatewayAddress); idx++ {
			if currentAddress[idx] == gatewayAddress[idx] {
				continue
			}
			if idx >= searchLongest {
				searchIdx = i
				searchLongest = idx
				continue NEXT
			}
		}
	}
	if searchIdx != -1 {
		defaultListeningAddress = detectedGatewayAddresses[searchIdx]
		Info("Found default ip address: %s\n", defaultListeningAddress)
		return defaultListeningAddress, detectedGatewayInterfaces
	}
	defaultListeningAddress = detectedGatewayAddresses[0]
	return defaultListeningAddress, detectedGatewayInterfaces
}

func GetLanListeningAddresses() ([]string, []int64) {
	return detectedGatewayAddresses, detectedGatewayInterfaces
}
