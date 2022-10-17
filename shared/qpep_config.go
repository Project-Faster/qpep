package shared

import (
	"errors"
	"fmt"
	"io"
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
ackDelay: 25
congestion: 4
decimate: 4
minBeforeDecimation: 100
gateway: 198.18.0.254
port: 443
apiport: 444
listenaddress: 0.0.0.0
listenport: 9443
multistream: true
verbose: false
varAckDelay: 0
threads: 1
`
)

type QPepConfigType struct {
	Acks             int    `yaml:"acks"`
	AckDelay         int    `yaml:"ackDelay"`
	Congestion       int    `yaml:"congestion"`
	Decimate         int    `yaml:"decimate"`
	DelayDecimate    int    `yaml:"minBeforeDecimation"`
	GatewayHost      string `yaml:"gateway"`
	GatewayPort      int    `yaml:"port"`
	GatewayAPIPort   int    `yaml:"apiport"`
	ListenHost       string `yaml:"listenaddress"`
	ListenPort       int    `yaml:"listenport"`
	MultiStream      bool   `yaml:"multistream"`
	Verbose          bool   `yaml:"verbose"`
	VarAckDelay      int    `yaml:"varAckDelay"`
	WinDivertThreads int    `yaml:"threads"`
}

var (
	QPepConfig                QPepConfigType
	defaultListeningAddress   string
	detectedGatewayInterfaces []int64
)

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

func init() {
	var err error
	detectedGatewayInterfaces, err = getRouteGatewayInterfaces()

	Info("gateway interfaces: %v\n", detectedGatewayInterfaces)

	if err != nil {
		panic(err)
	}
}

func GetDefaultLanListeningAddress(currentAddress string) (string, []int64) {
	if len(defaultListeningAddress) > 0 {
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	if !strings.HasPrefix(currentAddress, "0.") && !strings.HasPrefix(currentAddress, "127.") {
		return currentAddress, detectedGatewayInterfaces
	}

	Info("WARNING: Detected invalid listening ip address, trying to autodetect the default route...\n")

	defaultIP, err := gateway.DiscoverInterface()
	if err != nil {
		panic(fmt.Sprint("PANIC: Could not discover default lan address and the requested one is not suitable, error: %v\n", err))
		return currentAddress, detectedGatewayInterfaces
	}

	defaultListeningAddress = defaultIP.String()
	Info("Found default ip address: %s\n", defaultListeningAddress)
	return defaultListeningAddress, detectedGatewayInterfaces
}
