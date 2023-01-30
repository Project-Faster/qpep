package shared

import (
	"bou.ke/monkey"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

const TEST_STRING_CONFIG = `
maxretries: "10"
gateway: "198.18.0.254"
port: "443"
apiport: "444"
listenaddress: "0.0.0.0"
listenport: "9443"
multistream: "true"
verbose: "false"
preferproxy: "false"
threads: "4"
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
	assert.Equal(s.T(), newValues, "{10 198.18.0.254 443 444 0.0.0.0 9443 true false false 4 0 0 0 0 0 0}")
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
	assert.Equal(s.T(), newValues, "{10 198.18.0.254 443 444 0.0.0.0 9443 true false false 4 0 0 0 0 0 0}")
}

func (s *QPepConfigSuite) TestGetConfigurationPaths() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectConfFile := filepath.Join(expectConfDir, CONFIG_FILENAME)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	_ = os.RemoveAll(expectConfDir)

	confDir, confFile, confUserFile := GetConfigurationPaths()
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

func (s *QPepConfigSuite) TestGetConfigurationPaths_Error() {
	guard := monkey.Patch(os.Executable, func() (string, error) {
		return "<test-error>", errors.New("<test-error>")
	})
	defer guard.Unpatch()

	assert.Panics(s.T(), func() {
		GetConfigurationPaths()
	})
}

func (s *QPepConfigSuite) TestReadConfiguration_WithoutUserConfig() {
	assert.Nil(s.T(), ReadConfiguration(true))

	assert.NotNil(s.T(), QPepConfig)
	configValues := fmt.Sprintf("%v", QPepConfig)
	assert.Equal(s.T(), configValues, "{10 198.18.0.254 443 444 0.0.0.0 9443 true false false 4 10 25 4 100 0 4}")
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
	assert.Equal(s.T(), configValues, "{10 198.18.0.254 9090 444 0.0.0.0 9443 true false false 4 10 25 4 100 0 4}")
}

func (s *QPepConfigSuite) TestReadConfiguration_Panic() {
	guard := monkey.Patch(GetConfigurationPaths, func() (string, string, string) {
		panic("test")
	})
	defer guard.Unpatch()

	assert.NotPanics(s.T(), func() {
		assert.NotNil(s.T(), ReadConfiguration(false))
	})
}

func (s *QPepConfigSuite) TestReadConfiguration_createFileIfAbsentErrorMain() {
	guard := monkey.Patch(createFileIfAbsent, func(string, bool) (*os.File, error) {
		return nil, errors.New("<test-error>")
	})
	defer guard.Unpatch()

	assert.NotNil(s.T(), ReadConfiguration(true))
}

func (s *QPepConfigSuite) TestReadConfiguration_errorMainReadFile() {
	guard := monkey.Patch(io.ReadAll, func(io.Reader) ([]byte, error) {
		return nil, errors.New("<test-error>")
	})
	defer guard.Unpatch()

	assert.NotNil(s.T(), ReadConfiguration(true))
}

func (s *QPepConfigSuite) TestReadConfiguration_errorMainFailedUnmarshal() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	_ = os.Mkdir(expectConfDir, 0777)
	expectConfFile := filepath.Join(expectConfDir, CONFIG_FILENAME)
	_ = ioutil.WriteFile(expectConfFile, []byte("port: 9090\nport: 9090"), 0666)

	assert.NotNil(s.T(), ReadConfiguration(true))
}

func (s *QPepConfigSuite) TestReadConfiguration_createFileIfAbsentErrorUserFile() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	var guard *monkey.PatchGuard
	guard = monkey.Patch(createFileIfAbsent, func(reqFile string, b bool) (*os.File, error) {
		if reqFile == expectUserFile {
			return nil, errors.New("<test-error>")
		}
		guard.Unpatch()
		defer guard.Restore()

		return createFileIfAbsent(reqFile, b)
	})
	defer guard.Unpatch()

	// error is actually ignored if custom file cannot be created
	assert.Nil(s.T(), ReadConfiguration(false))
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

func (s *QPepConfigSuite) TestWriteConfigurationOverrideFile_PanicError() {
	guard := monkey.Patch(GetConfigurationPaths, func() (string, string, string) {
		panic("<test-error>")
	})
	defer guard.Unpatch()

	WriteConfigurationOverrideFile(nil)
}

func (s *QPepConfigSuite) TestWriteConfigurationOverrideFile_MarshalError() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	guard := monkey.Patch(yaml.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("<error>")
	})
	defer guard.Unpatch()

	WriteConfigurationOverrideFile(map[string]string{
		"test": "10",
	})
	_, err := os.Stat(expectUserFile)
	assert.Nil(s.T(), err)
}
