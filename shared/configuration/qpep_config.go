/*
 * This file in shared package handles the global configuration coming from yaml files.
 * It loads the two files "config.yaml" and "config.user.yaml" from the "config" folder
 * (in the binary directory).
 * The first file is required and will be created with default values if it does not exist.
 * The values in the user file (if present) will be merged with the main configuration and
 * overrides it.
 */
package configuration

import (
	"errors"
	"fmt"
	"github.com/Project-Faster/qpep/shared/logger"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
)

const (
	// DEFAULT_REDIRECT_RETRIES Value of default number of total tries for a connection before terminating
	DEFAULT_REDIRECT_RETRIES = 15
	// CONFIG_FILENAME Name of the main configuration file of the qpep service
	CONFIG_FILENAME = "qpep.yml"
	// CONFIG_OVERRIDE_FILENAME Name of the yaml configuration file for overrides by the tray
	CONFIG_OVERRIDE_FILENAME = "qpep.user.yml"
	// CONFIG_PATH Directory name for the configuration files
	CONFIG_PATH = "config"
	// LOGS_PATH Directory name for the log files
	LOGS_PATH = "log"
	// WEBGUI_URL URL of the web gui served by the service
	WEBGUI_URL = "http://127.0.0.1:%d/index?mode=%s&port=%d"
)

var (
	// QPepConfig Global variable for the configuration loaded from the qpep.yml file
	// with optional overrides from the qpep.user.yml configuration file
	QPepConfig QPepConfigType
)

// QPepConfigType Struct that represents all the parameters than influence the workings
// of the service and are loaded from the yaml main configuration file plus the additional
// user yaml file that overrides those values
type QPepConfigType struct {
	// Client (yaml:client) Definition of the address values used when in client mode
	Client *ClientDefinition `yaml:"client"`

	// Server (yaml:server) Definition of the address values used when in server mode
	Server *ServerDefinition `yaml:"server"`

	// Security (yaml:security) Definition of the certificate files used when establishing connections
	Security *CertDefinition `yaml:"security"`

	// Protocol (yaml:protocol) Definition of the selection of algorithms used by the QUIC backend
	Protocol *ProtocolDefinition `yaml:"protocol"`

	// General (yaml:general) Definition of generic parameters to change the behavior of the service
	General *GeneralDefinition `yaml:"general"`

	// Limits (yaml:limits) Declares the incoming and outgoing speed limits for clients and destination addresses
	Limits *LimitsDefinition `yaml:"limits"`

	// Analytics (yaml:analytics) Declares the configuration for server-side analytics collectìon, ignored for client, default disabled
	Analytics *AnalyticsDefinition `yaml:"analytics"`

	// Debug (yaml:debug) Definition of debug switches used during development, do not use in other contexts
	Debug *DebugDefinition `yaml:"debug"`
}

// Merge method updates all the fields of the configuration with the new configuration instance
func (q *QPepConfigType) Merge(r *QPepConfigType) {
	if r == nil {
		return
	}

	if q.Client == nil && r.Client != nil {
		q.Client = &ClientDefinition{}
	}
	if q.Server == nil && r.Server != nil {
		q.Server = &ServerDefinition{}
	}
	if q.Security == nil && r.Security != nil {
		q.Security = &CertDefinition{}
	}
	if q.Protocol == nil && r.Protocol != nil {
		q.Protocol = &ProtocolDefinition{}
	}
	if q.General == nil && r.General != nil {
		q.General = &GeneralDefinition{}
	}
	if q.Limits == nil && r.Limits != nil {
		q.Limits = &LimitsDefinition{}
	}
	if q.Analytics == nil && r.Analytics != nil {
		q.Analytics = &AnalyticsDefinition{}
	}
	if q.Debug == nil && r.Debug != nil {
		q.Debug = &DebugDefinition{}
	}

	q.Client.merge(r.Client)
	q.Server.merge(r.Server)
	q.Security.merge(r.Security)
	q.Protocol.merge(r.Protocol)
	q.General.merge(r.General)
	q.Limits.merge(r.Limits)
	q.Analytics.merge(r.Analytics)
	q.Debug.merge(r.Debug)
}

// GetConfigurationPaths returns the current paths for handling the configuration files, creating them if those don't exist:
// configuration directory, configuration filename and the configuration user filename
func GetConfigurationPaths() (confDir string, confFile string, confUserFile, logsDir string) {
	basedir, err := os.Executable()
	if err != nil {
		logger.Panic("Could not find executable: %s", err)
	}

	confDir = filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	if _, err := os.Stat(confDir); err != nil {
		err = os.Mkdir(confDir, 0777)
		if err != nil {
			logger.Panic("Error creating configuration folder: %v\n", err)
		}
	}

	confFile = filepath.Join(confDir, CONFIG_FILENAME)
	if _, err := os.Stat(confFile); err != nil {
		defaultData, _ := yaml.Marshal(&DefaultConfig)

		err = os.WriteFile(confFile, defaultData, 0777)
		if err != nil {
			logger.Panic("Error creating main configuration file: %v\n", err)
		}
	}

	confUserFile = filepath.Join(confDir, CONFIG_OVERRIDE_FILENAME)
	if _, err := os.Stat(confUserFile); err != nil {
		err = os.WriteFile(confUserFile, []byte(``), 0777)
		if err != nil {
			logger.Error("Error creating user configuration file: %v\n", err)
		}
	}

	logsDir = filepath.Join(filepath.Dir(basedir), LOGS_PATH)
	if _, err := os.Stat(logsDir); err != nil {
		err = os.Mkdir(logsDir, 0777)
		if err != nil {
			logger.Panic("Error creating logs folder: %v\n", err)
		}
	}

	return confDir, confFile, confUserFile, logsDir
}

// ReadConfiguration method loads the global configuration from the yaml files, if the _ignoreCustom_ value is true
// then only the main file is loaded, ignoring the user one, if false then the user file is loaded and its config values
// Merge the main ones
func ReadConfiguration(ignoreCustom bool) (outerr error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v", err)
			debug.PrintStack()
			outerr = errors.New(fmt.Sprintf("%v", err))
		}
		logger.Info("Configuration Loaded")
	}()

	// reset previous configuration
	QPepConfig = QPepConfigType{}
	QPepConfig.Merge(&DefaultConfig)

	_, confFile, userConfFile, _ := GetConfigurationPaths()

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

	// cache clients keys
	LoadAddressSpeedLimitMap(QPepConfig.Limits.Incoming, true)

	// cache destinations keys
	LoadAddressSpeedLimitMap(QPepConfig.Limits.Outgoing, false)

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

	var userConfig QPepConfigType
	dataCustom, _ := io.ReadAll(fUser)
	if err = yaml.Unmarshal(dataCustom, &userConfig); err == nil {
		logger.Info("Merge %v", userConfig)

		// actual Merge of main configuration and user one
		QPepConfig.Merge(&userConfig)
	}
	return nil
}

// WriteConfigurationOverrideFile method writes the indicated map of values to the user yaml configuration file
// warning: can potentially contain values which are not recognized by the configuration
func WriteConfigurationOverrideFile(override QPepConfigType) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v", err)
			debug.PrintStack()
		}
	}()

	_, _, userConfFile, _ := GetConfigurationPaths()

	// create base config if it does not exist
	f, _ := createFileIfAbsent(userConfFile, true)
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()

	data, err := yaml.Marshal(override)
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
