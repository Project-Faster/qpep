package shared

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/parvit/qpep/logger"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
)

// QLogWriter struct used by quic-go package to dump debug information
// abount quic connections
type QLogWriter struct {
	*bufio.Writer
}

// Close method flushes the data to internal writer
func (mwc *QLogWriter) Close() error {
	// Noop
	return mwc.Writer.Flush()
}

const (
	// DEFAULT_REDIRECT_RETRIES Value of default number of total tries for a connection before terminating
	DEFAULT_REDIRECT_RETRIES = 15
	// CONFIG_FILENAME Name of the main configuration file of the qpep service
	CONFIG_FILENAME = "qpep.yml"
	// CONFIG_OVERRIDE_FILENAME Name of the yaml configuration file for overrides by the tray
	CONFIG_OVERRIDE_FILENAME = "qpep.user.yml"
	// CONFIG_PATH Directory name for the configuration files
	CONFIG_PATH = "config"
	// WEBGUI_URL URL of the web gui served by the service
	WEBGUI_URL = "http://127.0.0.1:%d/index?mode=%s&port=%d"

	// DEFAULT_CONFIG Default yaml configuration written if not found
	DEFAULT_CONFIG = `
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
	// QPepConfig Global variable for the configuration loaded from the file, with
	// optional overrides from the user configuration file
	QPepConfig QPepConfigType
)

// QPepConfigType Struct that represents all the parameters than influence the workings
// of the service and are loaded from the yaml main configuration file plus the additional
// user yaml file that overrides those values
type QPepConfigType struct {
	// MaxConnectionRetries (yaml:maxretries) integer value for maximum number of connection tries to server before
	// stopping (half of these will be with diverter system, the other half with proxy)
	MaxConnectionRetries int `yaml:"maxretries"`
	// GatewayHost (yaml:gateway) Address of gateway qpep server for opening quic connections
	GatewayHost string `yaml:"gateway"`
	// GatewayPort (yaml:port) Port on which the gateway qpep server listens for quic connections
	GatewayPort int `yaml:"port"`
	// GatewayAPIPort (yaml:apiport) Port on which the gateway qpep server listens for TCP API requests
	GatewayAPIPort int `yaml:"apiport"`
	// ListenHost (yaml:listenaddress) Address on which the local instance (client or server) listens for incoming connections
	// if indicates subnet 0. or 127. it will try to autodetect a good ip available
	ListenHost string `yaml:"listenaddress"`
	// ListenPort (yaml:listenport) Port where diverter or proxy will try to redirect the local tcp connections
	ListenPort int `yaml:"listenport"`
	// MultiStream (yaml:multistream) Indicates if MultiStream option for quic-go should be enabled or not
	MultiStream bool `yaml:"multistream"`
	// PreferProxy (yaml:preferproxy) If true the first half of retries will use the proxy system instead of diverter
	PreferProxy bool `yaml:"preferproxy"`
	// Verbose (yaml:verbose) Activates more verbose output than normal
	Verbose bool `yaml:"verbose"`
	// WinDivertThreads (yaml:threads) Indicates the number of threads that the diverter should use to handle packages
	WinDivertThreads int `yaml:"threads"`

	// -- Unused values -- //

	// Acks unused currently
	Acks int `yaml:"acks"`
	// AckDelay unused currently
	AckDelay int `yaml:"ackdelay"`
	// Decimate unused currently
	Decimate int `yaml:"decimate"`
	// DelayDecimate unused currently
	DelayDecimate int `yaml:"decimatetime"`
	// VarAckDelay unused currently
	VarAckDelay int `yaml:"varackdelay"`

	// Congestion unused but probably will have an implementation soon
	Congestion int `yaml:"congestion"`

	// -- Unused values -- //
}

// rawConfigType struct that allows to decode and overwrite the main configuration
type rawConfigType map[string]interface{}

// updateIntField method udpates a variable pointer of int type if the variable name is contained in the map
// which is expected to contain a string value
func (r rawConfigType) updateIntField(field *int, name string) {
	if val, ok := r[name]; ok {
		switch v := val.(type) {
		case string:
			intValue, err := strconv.ParseInt(val.(string), 10, 64)
			if err == nil {
				*field = int(intValue)
				logger.Info("update int value [%s]: %d", name, intValue)
			}
			return
		case int:
			*field = v
			logger.Info("update int value [%s]: %d", name, *field)
			return
		}
	}
}

// updateIntField method udpates a variable pointer of string type if the variable name is contained in the map
// which is expected to contain a string value
func (r rawConfigType) updateStringField(field *string, name string) {
	if val, ok := r[name]; ok {
		*field = val.(string)
		logger.Info("update string value [%s]: %v", name, val)
	}
}

// updateIntField method udpates a variable pointer of boolean type if the variable name is contained in the map
// which is expected to contain a string value
func (r rawConfigType) updateBoolField(field *bool, name string) {
	if val, ok := r[name]; ok {
		switch v := val.(type) {
		case string:
			boolValue, err := strconv.ParseBool(val.(string))
			if err == nil {
				*field = boolValue
				logger.Info("update int value [%s]: %v", name, boolValue)
			}
			return
		case bool:
			*field = v
			logger.Info("update int value [%s]: %v", name, *field)
			return
		}
	}
}

// override method updates all the fields of the configuration with the rawConfigType map
func (q *QPepConfigType) override(r rawConfigType) {
	//r.updateIntField(&q.Acks, "acks")
	//r.updateIntField(&q.AckDelay, "ackdelay")
	//r.updateIntField(&q.Congestion, "congestion")
	//r.updateIntField(&q.Decimate, "decimate")
	//r.updateIntField(&q.DelayDecimate, "decimatetime")
	//r.updateIntField(&q.VarAckDelay, "varackdelay")

	r.updateIntField(&q.MaxConnectionRetries, "maxretries")
	r.updateStringField(&q.GatewayHost, "gateway")
	r.updateIntField(&q.GatewayPort, "port")
	r.updateIntField(&q.GatewayAPIPort, "apiport")
	r.updateStringField(&q.ListenHost, "listenaddress")
	r.updateIntField(&q.ListenPort, "listenport")
	r.updateBoolField(&q.MultiStream, "multistream")
	r.updateBoolField(&q.PreferProxy, "preferproxy")
	r.updateBoolField(&q.Verbose, "verbose")
	r.updateIntField(&q.WinDivertThreads, "threads")
}

// GetConfigurationPaths returns the current paths for handling the configuration files, creating them if those don't exist:
// configuration directory, configuration filename and the configuration override filename
func GetConfigurationPaths() (string, string, string) {
	basedir, err := os.Executable()
	if err != nil {
		logger.Panic("Could not find executable: %s", err)
	}

	confDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	if _, err := os.Stat(confDir); err != nil {
		err = os.Mkdir(confDir, 0777)
		if err != nil {
			logger.Panic("Error creating configuration folder: %v\n", err)
		}
	}

	confFile := filepath.Join(confDir, CONFIG_FILENAME)
	if _, err := os.Stat(confFile); err != nil {
		err = os.WriteFile(confFile, []byte(DEFAULT_CONFIG), 0777)
		if err != nil {
			logger.Panic("Error creating main configuration file: %v\n", err)
		}
	}

	confUserFile := filepath.Join(confDir, CONFIG_OVERRIDE_FILENAME)
	if _, err := os.Stat(confUserFile); err != nil {
		err = os.WriteFile(confUserFile, []byte(`\n`), 0777)
		if err != nil {
			logger.Error("Error creating user configuration file: %v\n", err)
		}
	}

	return confDir, confFile, confUserFile
}

// ReadConfiguration method loads the global configuration from the yaml files, if the _ignoreCustom_ value is true
// then only the main file is loaded, ignoring the user one, if false then the user file is loaded and its config values
// override the main ones
func ReadConfiguration(ignoreCustom bool) (outerr error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v", err)
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

	// check if custom file needs to be ignored
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

		// actual merge of main configuration and user one
		QPepConfig.override(userConfig)
	}
	return nil
}

// WriteConfigurationOverrideFile method writes the indicated map of values to the user yaml configuration file
// warning: can potentially contain values which are not recognized by the configuration
func WriteConfigurationOverrideFile(values map[string]string) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v", err)
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

// createFileIfAbsent method creates a file for writing and optionally allows to truncate it by specifying the
// to true the _truncate_ parameter
func createFileIfAbsent(fileToCheck string, truncate bool) (*os.File, error) {
	var flags = os.O_RDWR | os.O_CREATE
	if truncate {
		flags = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	}
	return os.OpenFile(fileToCheck, flags, 0666)
}
