package shared

import (
	"fmt"
	"github.com/Project-Faster/monkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

const TEST_STRING_CONFIG = `
maxretries: "10"
gateway: "198.18.0.254"
port: "1443"
apiport: "8444"
listenaddress: "0.0.0.0"
listenport: "9443"
backend: "quic-go"
ccalgorithm: "reno"
ccslowstart: "search"
certificate: "cert.pem"
certificate_key: "key.pem"
multistream: "true"
verbose: "false"
preferproxy: "false"
threads: "4"
`

const TEST_LIMITS_CONFIG = `
maxretries: 10
gateway: 198.18.0.254
port: 1443
apiport: 8444
listenaddress: 0.0.0.0
listenport: 9443
backend: quic-go
ccalgorithm: reno
ccslowstart: search
certificate: cert.pem
certificate_key: key.pem
multistream: true
verbose: false
preferproxy: false
threads: 4

limits:
  clients:
    192.168.1.1/25: 100K

  destinations:
    wikipedia.com: 0
    192.168.1.128/25: 200K
`

const TEST_BROKER_CONFIG = `
maxretries: 10
gateway: 198.18.0.254
port: 1443
apiport: 8444
listenaddress: 0.0.0.0
listenport: 9443
backend: quic-go
ccalgorithm: reno
ccslowstart: search
certificate: cert.pem
certificate_key: key.pem
multistream: true
verbose: false
preferproxy: false
threads: 4

analytics:
  enabled: true
  topic: qpep-server-1/data
  address: 192.168.1.9
  port: 1883
  protocol: tcp
`

var testCheckFields = []string{
	"maxretries", "gateway", "port", "listenaddress", "listenport",
	"multistream", "preferproxy", "verbose", "threads",
}

func TestQPepConfig(t *testing.T) {
	var q QPepConfigSuite
	suite.Run(t, &q)
}

type QPepConfigSuite struct{ suite.Suite }

func (s *QPepConfigSuite) BeforeTest() {
}

func (s *QPepConfigSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()

	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)

	_ = os.RemoveAll(expectConfDir)
}

func (s *QPepConfigSuite) TestRawConfigType_Unmarshal() {
	var r rawConfigType
	assert.Nil(s.T(), yaml.Unmarshal([]byte(DEFAULT_CONFIG), &r))

	for _, k := range testCheckFields {
		_, ok := r[k]
		assert.True(s.T(), ok)
	}
}

func (s *QPepConfigSuite) TestRawConfigType_OverrideRealType() {
	var r rawConfigType
	assert.Nil(s.T(), yaml.Unmarshal([]byte(DEFAULT_CONFIG), &r))

	var config QPepConfigType
	var prevValues = fmt.Sprintf("%v", config)

	config.override(r)

	var newValues = fmt.Sprintf("%v", config)
	s.T().Logf("config: %v == %v\n", prevValues, newValues)

	assert.NotEqual(s.T(), prevValues, newValues)
	assert.Equal(s.T(), "{10 198.18.0.254 1443 8444 quic-go 0.0.0.0 9443 true true false 4 server_cert.pem server_key.pem reno search {map[] map[]} {false  0  } 0 0 0 0 0}",
		newValues)
}

func (s *QPepConfigSuite) TestRawConfigType_OverrideStringType() {
	var r rawConfigType
	assert.Nil(s.T(), yaml.Unmarshal([]byte(TEST_STRING_CONFIG), &r))

	var config QPepConfigType
	var prevValues = fmt.Sprintf("%v", config)

	config.override(r)

	var newValues = fmt.Sprintf("%v", config)
	s.T().Logf("config: %v == %v\n", prevValues, newValues)

	assert.NotEqual(s.T(), prevValues, newValues)
	assert.Equal(s.T(), "{10 198.18.0.254 1443 8444 quic-go 0.0.0.0 9443 true false false 4 cert.pem key.pem reno search {map[] map[]} {false  0  } 0 0 0 0 0}",
		newValues)
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

	assert.NotNil(s.T(), QPepConfig)
	configValues := fmt.Sprintf("%v", QPepConfig)
	assert.Equal(s.T(), "{10 198.18.0.254 1443 8444 quic-go 0.0.0.0 9443 true true false 4 server_cert.pem server_key.pem reno search {map[] map[]} {false  0  } 10 25 4 100 0}",
		configValues)
}

func (s *QPepConfigSuite) TestReadConfiguration_WithUserConfigOverride() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	_ = os.Mkdir(expectConfDir, 0777)
	expectConfFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)
	_ = ioutil.WriteFile(expectConfFile, []byte("port: 9090\n"), 0666)

	assert.Nil(s.T(), ReadConfiguration(false))

	assert.NotNil(s.T(), QPepConfig)
	configValues := fmt.Sprintf("%v", QPepConfig)
	assert.Equal(s.T(), "{10 198.18.0.254 9090 8444 quic-go 0.0.0.0 9443 true true false 4 server_cert.pem server_key.pem reno search {map[] map[]} {false  0  } 10 25 4 100 0}",
		configValues)
}

func (s *QPepConfigSuite) TestReadConfiguration_WithLimitsConfig() {
	_, f, _, _ := GetConfigurationPaths()
	_ = ioutil.WriteFile(f, []byte(TEST_LIMITS_CONFIG), 0777)

	assert.Nil(s.T(), ReadConfiguration(true))

	assert.NotNil(s.T(), QPepConfig)
	configValues := fmt.Sprintf("%v", QPepConfig)
	assert.Equal(s.T(), "{10 198.18.0.254 1443 8444 quic-go 0.0.0.0 9443 true false false 4 cert.pem key.pem reno search {map[192.168.1.1/25:100K] map[192.168.1.128/25:200K wikipedia.com:0]} {false  0  } 0 0 0 0 0}",
		configValues)
}

func (s *QPepConfigSuite) TestReadConfiguration_WithBrokerConfig() {
	_, f, _, _ := GetConfigurationPaths()
	_ = ioutil.WriteFile(f, []byte(TEST_BROKER_CONFIG), 0777)

	assert.Nil(s.T(), ReadConfiguration(true))

	assert.NotNil(s.T(), QPepConfig)
	configValues := fmt.Sprintf("%v", QPepConfig)
	assert.Equal(s.T(), "{10 198.18.0.254 1443 8444 quic-go 0.0.0.0 9443 true false false 4 cert.pem key.pem reno search {map[] map[]} {true 192.168.1.9 1883 tcp qpep-server-1/data} 0 0 0 0 0}",
		configValues)
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

	WriteConfigurationOverrideFile(map[string]string{
		"test": "10",
	})

	data, err := ioutil.ReadFile(expectUserFile)
	assert.Nil(s.T(), err)

	assertArrayEquals(s.T(), []byte("test: \"10\"\n"), data)
}

func (s *QPepConfigSuite) TestWriteConfigurationOverrideFile_createFileIfAbsentErrorUserFile() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	WriteConfigurationOverrideFile(nil)

	_, err := os.Stat(expectUserFile)
	assert.Nil(s.T(), err)
}
