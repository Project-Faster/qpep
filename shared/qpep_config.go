package shared

import (
	"bufio"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"

	"github.com/parvit/qpep/logger"
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
	QPepConfig QPepConfigType
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
			logger.Info("update int value [%s]: %d", name, intValue)
		}
	}
}
func (r rawConfigType) updateStringField(field *string, name string) {
	if val, ok := r[name]; ok {
		*field = val.(string)
		logger.Info("update string value [%s]: %v", name, val)
	}
}
func (r rawConfigType) updateBoolField(field *bool, name string) {
	if val, ok := r[name]; ok {
		boolValue, err := strconv.ParseBool(val.(string))
		if err == nil {
			*field = boolValue
			logger.Info("update bool value [%s]: %b", name, boolValue)
		}
	}
}

func (q *QPepConfigType) override(r rawConfigType) {
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

func GetConfigurationPaths() (string, string, string) {
	basedir, err := os.Executable()
	if err != nil {
		logger.Error("Could not find executable: %s", err)
	}

	confdir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	if _, err := os.Stat(confdir); errors.Is(err, os.ErrNotExist) {
		os.Mkdir(confdir, 0664)
	}

	confFile := filepath.Join(confdir, CONFIG_FILENAME)
	if _, err := os.Stat(confFile); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(confFile, []byte(DEFAULT_CONFIG), 0666)
	}

	confUserFile := filepath.Join(confdir, CONFIG_OVERRIDE_FILENAME)
	if _, err := os.Stat(confUserFile); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(confUserFile, []byte(`\n`), 0666)
	}

	return confdir, confFile, confUserFile
}

func ReadConfiguration(ignoreCustom bool) (outerr error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: ", err)
			debug.PrintStack()
			outerr = errors.New(fmt.Sprintf("%v", err))
		}
		logger.Info("Configuration Loaded")
	}()

	_, confFile, userConfFile := GetConfigurationPaths()

	// Read base config
	f, err := createFileIfAbsent(confFile, false)
	if err != nil {
		logger.Error("Could not read expected configuration file: %v", err)
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		logger.Error("Could not read expected configuration file: %v", err)
		return err
	}
	if err := yaml.Unmarshal(data, &QPepConfig); err != nil {
		logger.Error("Could not decode configuration file: %v", err)
		return err
	}

	if ignoreCustom {
		return nil
	}

	// Read overrides
	fUser, err := createFileIfAbsent(userConfFile, false)
	if err != nil {
		return nil
	}
	defer func() {
		if fUser != nil {
			_ = fUser.Close()
		}
	}()

	var userConfig rawConfigType
	dataCustom, _ := io.ReadAll(fUser)
	if err = yaml.Unmarshal(dataCustom, &userConfig); err == nil {
		logger.Info("override %v", userConfig)
		QPepConfig.override(userConfig)
	}
	return nil
}

func WriteConfigurationOverrideFile(values map[string]string) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: ", err)
			debug.PrintStack()
		}
	}()

	_, _, userConfFile := GetConfigurationPaths()

	// create base config if it does not exist
	f, _ := createFileIfAbsent(userConfFile, true)
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()

	if len(values) == 0 {
		return
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		logger.Error("Could not read expected configuration file: %v", err)
		return
	}
	_, _ = f.Write(data)
	return
}

func createFileIfAbsent(fileToCheck string, truncate bool) (*os.File, error) {
	var flags = os.O_RDWR | os.O_CREATE
	if truncate {
		flags = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	}
	return os.OpenFile(fileToCheck, flags, 0666)
}
