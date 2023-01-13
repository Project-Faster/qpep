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
	"strconv"
	"strings"

	"github.com/jackpal/gateway"
	"gopkg.in/yaml.v3"

	. "github.com/parvit/qpep/logger"
)

func init() {
	var err error
	detectedGatewayInterfaces, detectedGatewayAddresses, err = getRouteGatewayInterfaces()

	if err != nil {
		panic(err)
	}
}

type QLogWriter struct {
	*bufio.Writer
}

func (mwc *QLogWriter) Close() error {
	// Noop
	return mwc.Writer.Flush()
}

const (
	DEFAULT_REDIRECT_RETRIES = 15
	CONFIG_FILENAME          = "qpep.yml"
	CONFIG_OVERRIDE_FILENAME = "qpep.user.yml"
	CONFIG_PATH              = "config"
	WEBGUI_URL               = "http://127.0.0.1:%d/index?mode=%s&port=%d"
	DEFAULT_CONFIG           = `
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

var (
	QPepConfig                QPepConfigType
	UsingProxy                = false
	ProxyAddress              *url.URL
	defaultListeningAddress   string
	detectedGatewayInterfaces []int64
	detectedGatewayAddresses  []string
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

type rawConfigType map[string]interface{}

func (r rawConfigType) updateIntField(field *int, name string) {
	if val, ok := r[name]; ok {
		intValue, err := strconv.ParseInt(val.(string), 10, 64)
		if err == nil {
			*field = int(intValue)
		}
	}
}
func (r rawConfigType) updateStringField(field *string, name string) {
	if val, ok := r[name]; ok {
		*field = val.(string)
	}
}
func (r rawConfigType) updateBoolField(field *bool, name string) {
	if val, ok := r[name]; ok {
		boolValue, err := strconv.ParseBool(val.(string))
		if err == nil {
			*field = boolValue
		}
	}
}

func (q QPepConfigType) override(r rawConfigType) {
	r.updateIntField(&q.Acks, "acks")
	r.updateIntField(&q.AckDelay, "ackdelay")
	r.updateIntField(&q.Congestion, "congestion")
	r.updateIntField(&q.MaxConnectionRetries, "maxretries")
	r.updateIntField(&q.Decimate, "decimate")
	r.updateIntField(&q.DelayDecimate, "decimatetime")
	r.updateStringField(&q.GatewayHost, "gateway")
	r.updateIntField(&q.GatewayPort, "port")
	r.updateIntField(&q.GatewayAPIPort, "apiport")
	r.updateStringField(&q.ListenHost, "listenaddress")
	r.updateIntField(&q.ListenPort, "listenport")
	r.updateBoolField(&q.MultiStream, "multistream")
	r.updateBoolField(&q.PreferProxy, "preferproxy")
	r.updateBoolField(&q.Verbose, "verbose")
	r.updateIntField(&q.VarAckDelay, "varackdelay")
	r.updateIntField(&q.WinDivertThreads, "threads")
}

func GetConfigurationPath() string {
	basedir, err := os.Executable()
	if err != nil {
		Error("Could not find executable: %s", err)
	}

	confdir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	if _, err := os.Stat(confdir); errors.Is(err, os.ErrNotExist) {
		os.Mkdir(confdir, 0664)
	}
	return confdir
}

func ReadConfiguration(ignoreCustom bool) (outerr error) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: ", err)
			debug.PrintStack()
			outerr = errors.New(fmt.Sprintf("%v", err))
		}
	}()

	confdir := GetConfigurationPath()

	confFile := filepath.Join(confdir, CONFIG_FILENAME)
	if _, err := os.Stat(confFile); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(confFile, []byte(DEFAULT_CONFIG), 0664)
	}

	// Read base config
	f, err := os.Open(confFile)
	if err != nil {
		Error("Could not read expected configuration file: %v", err)
		return err
	}
	defer func() {
		_ = f.Close()
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

	if !ignoreCustom {
		// Read overrides
		userConfFile := filepath.Join(confdir, CONFIG_OVERRIDE_FILENAME)
		fUser, err := os.Open(userConfFile)
		if err == nil {
			defer func() {
				_ = fUser.Close()
			}()

			var userConfig rawConfigType
			dataCustom, err := io.ReadAll(fUser)
			if err == nil {
				if err = yaml.Unmarshal(dataCustom, &userConfig); err == nil {
					QPepConfig.override(userConfig)
				}
			}
		}
	}

	Info("Configuration Loaded")

	return nil
}

func WriteConfigurationOverrideFile(values map[string]string) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: ", err)
			debug.PrintStack()
		}
	}()

	confdir := GetConfigurationPath()

	confFile := filepath.Join(confdir, CONFIG_OVERRIDE_FILENAME)

	f, err := os.Create(confFile)
	if err != nil {
		Error("Could not read expected configuration file: %v", err)
		return
	}
	defer func() {
		_ = f.Close()
	}()

	if len(values) == 0 {
		return
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		Error("Could not read expected configuration file: %v", err)
		return
	}
	_, _ = f.Write(data)
	return
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
