package configuration

import (
	"github.com/Project-Faster/monkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

const TEST_LIMITS_CONFIG = `
limits:
  incoming:
    192.168.1.1/24: 100K

  outgoing:
    wikipedia.com: 0
    192.168.1.128/24: 200K
`

const TEST_BROKER_CONFIG = `
analytics:
  enabled: true
  topic: qpep-server-1/data
  address: 192.168.1.9
  port: 1883
  protocol: tcp
`

const TEST_DEFAULT_CONFIG = `
client:
  local_address: 0.0.0.0
  local_port: 9443
  gateway_address: 198.18.0.254
  gateway_port: 443

security:
  certificate: server_cert.pem

protocol:
  backend: quic-go
  buffersize: 512
  ccalgorithm: cubic
  ccslowstart: search

general:
  max_retries: 50
  use_multistream: true
  prefer_proxy: true
  verbose: false
  api_port: 444
`

func TestQPepConfig(t *testing.T) {
	var q QPepConfigSuite
	suite.Run(t, &q)
}

type QPepConfigSuite struct{ suite.Suite }

func (s *QPepConfigSuite) BeforeTest(_, _ string) {
	QPepConfig = QPepConfigType{}
}

func (s *QPepConfigSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()

	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)

	_ = os.RemoveAll(expectConfDir)
}

func (s *QPepConfigSuite) TestOverrideEmpty() {
	var readConfig QPepConfigType
	assert.Nil(s.T(), yaml.Unmarshal([]byte(``), &readConfig))

	var config QPepConfigType
	config.Merge(&readConfig)

	assertConfigEquals(s.T(), &readConfig, &config)
}

func (s *QPepConfigSuite) TestOverride() {
	var readConfig QPepConfigType
	assert.Nil(s.T(), yaml.Unmarshal([]byte(TEST_BROKER_CONFIG), &readConfig))

	var config QPepConfigType
	config.Merge(&readConfig)

	assertConfigEquals(s.T(), &readConfig, &config)
}

func (s *QPepConfigSuite) TestGetConfigurationPaths() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectConfFile := filepath.Join(expectConfDir, CONFIG_FILENAME)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	_ = os.RemoveAll(expectConfDir)

	confDir, confFile, confUserFile, _ := GetConfigurationPaths()
	assert.True(s.T(), len(confDir) > 0)
	assert.True(s.T(), len(confFile) > 0)
	assert.True(s.T(), len(confUserFile) > 0)

	assert.Equal(s.T(), expectConfDir, confDir)
	assert.Equal(s.T(), expectConfFile, confFile)
	assert.Equal(s.T(), expectUserFile, confUserFile)

	_, err := os.Stat(confDir)
	assert.Nil(s.T(), err)
	_, err = os.Stat(confFile)
	assert.Nil(s.T(), err)
	_, err = os.Stat(confUserFile)
	assert.Nil(s.T(), err)
}

func (s *QPepConfigSuite) TestReadConfiguration_WithoutUserConfig() {
	assert.Nil(s.T(), ReadConfiguration(true))

	assertConfigEquals(s.T(), &QPepConfig, &DefaultConfig)
}

func (s *QPepConfigSuite) TestReadConfiguration_WithUserConfigOverride() {
	_, f, fUser, _ := GetConfigurationPaths()
	_ = ioutil.WriteFile(f, []byte(TEST_DEFAULT_CONFIG), 0666)
	_ = ioutil.WriteFile(fUser, []byte(TEST_BROKER_CONFIG), 0666)

	assert.Nil(s.T(), ReadConfiguration(false))

	expectedRead := QPepConfigType{
		// base + merged
		Client: &ClientDefinition{
			LocalListeningAddress: "0.0.0.0",
			LocalListenPort:       9443,
			GatewayHost:           "198.18.0.254",
			GatewayPort:           443,
		},
		Security: &CertDefinition{
			Certificate: "server_cert.pem",
			PrivateKey:  "",
		},
		Protocol: &ProtocolDefinition{
			Backend:         "quic-go",
			BufferSize:      512,
			CCAlgorithm:     "cubic",
			CCSlowstartAlgo: "search",
		},
		General: &GeneralDefinition{
			MaxConnectionRetries: 50,
			WinDivertThreads:     4,
			MultiStream:          true,
			PreferProxy:          true,
			APIPort:              444,
			Verbose:              false,
		},
		Analytics: &AnalyticsDefinition{
			Enabled:        true,
			BrokerAddress:  "192.168.1.9",
			BrokerPort:     1883,
			BrokerProtocol: "tcp",
			BrokerTopic:    "qpep-server-1/data",
		},
		// from default
		Server: &ServerDefinition{
			LocalListeningAddress: "",
			LocalListenPort:       0,
		},
		Limits: &LimitsDefinition{
			Incoming: nil,
			Outgoing: nil,
		},
		Debug: &DebugDefinition{
			DumpPackets:  false,
			MaskRedirect: false,
		},
	}

	assertConfigEquals(s.T(), &expectedRead, &QPepConfig)
}

func (s *QPepConfigSuite) TestReadConfiguration_WithLimitsConfig() {
	_, f, _, _ := GetConfigurationPaths()
	_ = ioutil.WriteFile(f, []byte(TEST_LIMITS_CONFIG), 0777)

	assert.Nil(s.T(), QPepConfig.Limits)
	assert.Nil(s.T(), ReadConfiguration(true))

	assertConfigNotEquals(s.T(), &QPepConfig, &DefaultConfig)

	assert.NotNil(s.T(), QPepConfig.Limits)
	assert.Equal(s.T(), "100K", QPepConfig.Limits.Incoming["192.168.1.1/24"])
	assert.Equal(s.T(), "0", QPepConfig.Limits.Outgoing["wikipedia.com"])
	assert.Equal(s.T(), "200K", QPepConfig.Limits.Outgoing["192.168.1.128/24"])
}

func (s *QPepConfigSuite) TestReadConfiguration_WithBrokerConfig() {
	_, f, _, _ := GetConfigurationPaths()
	_ = ioutil.WriteFile(f, []byte(TEST_BROKER_CONFIG), 0777)

	assert.Nil(s.T(), QPepConfig.Analytics)
	assert.Nil(s.T(), ReadConfiguration(true))

	assertConfigNotEquals(s.T(), &QPepConfig, &DefaultConfig)

	assert.NotNil(s.T(), QPepConfig.Analytics)
	assert.True(s.T(), QPepConfig.Analytics.Enabled)
	assert.Equal(s.T(), "192.168.1.9", QPepConfig.Analytics.BrokerAddress)
	assert.Equal(s.T(), 1883, QPepConfig.Analytics.BrokerPort)
	assert.Equal(s.T(), "qpep-server-1/data", QPepConfig.Analytics.BrokerTopic)
	assert.Equal(s.T(), "tcp", QPepConfig.Analytics.BrokerProtocol)
}

func (s *QPepConfigSuite) TestReadConfiguration_errorMainFailedUnmarshal() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	_ = os.Mkdir(expectConfDir, 0777)
	expectConfFile := filepath.Join(expectConfDir, CONFIG_FILENAME)
	_ = ioutil.WriteFile(expectConfFile, []byte("port: 9090\nport: 9090"), 0666)

	assert.NotNil(s.T(), ReadConfiguration(true))
}

func (s *QPepConfigSuite) TestWriteConfigurationOverrideFile() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	WriteConfigurationOverrideFile(QPepConfigType{
		Debug: &DebugDefinition{
			DumpPackets:  true,
			MaskRedirect: true,
		},
	})

	data, err := ioutil.ReadFile(expectUserFile)
	assert.Nil(s.T(), err)

	overrideConfig := QPepConfigType{}
	err = yaml.Unmarshal(data, &overrideConfig)
	assert.Nil(s.T(), err)

	assert.NotNil(s.T(), overrideConfig.Debug)
	assert.Nil(s.T(), overrideConfig.Client)
	assert.Nil(s.T(), overrideConfig.Server)
	assert.Nil(s.T(), overrideConfig.General)
	assert.Nil(s.T(), overrideConfig.Security)
	assert.Nil(s.T(), overrideConfig.Protocol)
	assert.Nil(s.T(), overrideConfig.Limits)
	assert.Nil(s.T(), overrideConfig.Analytics)
}

func (s *QPepConfigSuite) TestWriteConfigurationOverrideFile_createFileIfAbsentErrorUserFile() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	WriteConfigurationOverrideFile(QPepConfigType{})

	_, err := os.Stat(expectUserFile)
	assert.Nil(s.T(), err)
}

// ---- helpers ---- //
func assertConfigNotEquals(t *testing.T, a, b *QPepConfigType) {
	res := implAssertConfigEquals(t, a, b)

	assert.False(t, res)
}

func assertConfigEquals(t *testing.T, a, b *QPepConfigType) {
	res := implAssertConfigEquals(t, a, b)

	assert.True(t, res)
}

func implAssertConfigEquals(t *testing.T, a, b *QPepConfigType) bool {
	t.Helper()

	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if !((a.Client != nil && b.Client != nil) || (a.Client == nil && b.Client == nil)) {
		return false
	}
	if !((a.Server != nil && b.Server != nil) || (a.Server == nil && b.Server == nil)) {
		return false
	}
	if !((a.Security != nil && b.Security != nil) || (a.Security == nil && b.Security == nil)) {
		return false
	}
	if !((a.Protocol != nil && b.Protocol != nil) || (a.Protocol == nil && b.Protocol == nil)) {
		return false
	}
	if !((a.General != nil && b.General != nil) || (a.General == nil && b.General == nil)) {
		return false
	}
	if !((a.Limits != nil && b.Limits != nil) || (a.Limits == nil && b.Limits == nil)) {
		return false
	}
	if !((a.Analytics != nil && b.Analytics != nil) || (a.Analytics == nil && b.Analytics == nil)) {
		return false
	}
	if !((a.Debug != nil && b.Debug != nil) || (a.Debug == nil && b.Debug == nil)) {
		return false
	}

	if a.Client != nil {
		aClient := a.Client
		bClient := b.Client
		assert.NotNil(t, bClient)
		if aClient.LocalListeningAddress != bClient.LocalListeningAddress {
			return false
		}
		if aClient.LocalListenPort != bClient.LocalListenPort {
			return false
		}
		if aClient.GatewayHost != bClient.GatewayHost {
			return false
		}
		if aClient.GatewayPort != bClient.GatewayPort {
			return false
		}
	}
	if a.Server != nil {
		aServer := a.Server
		bServer := b.Server
		assert.NotNil(t, bServer)
		if aServer.LocalListeningAddress != bServer.LocalListeningAddress {
			return false
		}
		if aServer.LocalListenPort != bServer.LocalListenPort {
			return false
		}
	}
	if a.Security != nil {
		aSecurity := a.Security
		bSecurity := b.Security
		assert.NotNil(t, bSecurity)
		if aSecurity.Certificate != bSecurity.Certificate {
			return false
		}
		if aSecurity.PrivateKey != bSecurity.PrivateKey {
			return false
		}
	}
	if a.Protocol != nil {
		aProtocol := a.Protocol
		bProtocol := b.Protocol
		assert.NotNil(t, bProtocol)
		if aProtocol.Backend != bProtocol.Backend {
			return false
		}
		if aProtocol.BufferSize != bProtocol.BufferSize {
			return false
		}
		if aProtocol.CCAlgorithm != bProtocol.CCAlgorithm {
			return false
		}
		if aProtocol.CCSlowstartAlgo != bProtocol.CCSlowstartAlgo {
			return false
		}
	}
	if a.General != nil {
		aGeneral := a.General
		bGeneral := b.General
		assert.NotNil(t, bGeneral)
		if aGeneral.MaxConnectionRetries != bGeneral.MaxConnectionRetries {
			return false
		}
		if aGeneral.WinDivertThreads != bGeneral.WinDivertThreads {
			return false
		}
		if aGeneral.MultiStream != bGeneral.MultiStream {
			return false
		}
		if aGeneral.PreferProxy != bGeneral.PreferProxy {
			return false
		}
		if aGeneral.APIPort != bGeneral.APIPort {
			return false
		}
		if aGeneral.Verbose != bGeneral.Verbose {
			return false
		}
	}
	if a.Debug != nil {
		aDebug := a.Debug
		bDebug := b.Debug
		assert.NotNil(t, bDebug)
		if aDebug.DumpPackets != bDebug.DumpPackets {
			return false
		}
		if aDebug.MaskRedirect != bDebug.MaskRedirect {
			return false
		}
	}
	if a.Analytics != nil {
		aAnalytics := a.Analytics
		bAnalytics := b.Analytics
		assert.NotNil(t, bAnalytics)
		if aAnalytics.Enabled != bAnalytics.Enabled {
			return false
		}
		if aAnalytics.BrokerAddress != bAnalytics.BrokerAddress {
			return false
		}
		if aAnalytics.BrokerPort != bAnalytics.BrokerPort {
			return false
		}
		if aAnalytics.BrokerProtocol != bAnalytics.BrokerProtocol {
			return false
		}
		if aAnalytics.BrokerTopic != bAnalytics.BrokerTopic {
			return false
		}
	}
	if a.Limits != nil {
		aLimits := a.Limits
		bLimits := b.Limits
		assert.NotNil(t, bLimits)

		if !mapEquals(aLimits.Incoming, bLimits.Incoming) || !mapEquals(aLimits.Outgoing, bLimits.Outgoing) {
			return false
		}
	}

	return true
}

func mapEquals(mapA, mapB map[string]string) bool {
	keysA := make([]string, 0)
	for k, _ := range mapA {
		keysA = append(keysA, k)
	}
	keysB := make([]string, 0)
	for k, _ := range mapB {
		keysB = append(keysB, k)
	}
	if len(keysA) != len(keysB) {
		return false
	}

	for _, k := range keysA {
		val, ok := mapB[k]
		if !ok || mapA[k] != val {
			return false
		}
	}
	return true
}
