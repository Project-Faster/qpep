//go:build !arm64

package configuration

import (
	"errors"
	"github.com/Project-Faster/monkey"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
)

func (s *QPepConfigSuite) TestGetConfigurationPaths_Error() {
	guard := monkey.Patch(os.Executable, func() (string, error) {
		return "<test-error>", errors.New("<test-error>")
	})
	defer guard.Unpatch()

	assert.Panics(s.T(), func() {
		GetConfigurationPaths()
	})
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

func (s *QPepConfigSuite) TestReadConfiguration_Panic() {
	guard := monkey.Patch(GetConfigurationPaths, func() (string, string, string, string) {
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

func (s *QPepConfigSuite) TestWriteConfigurationOverrideFile_PanicError() {
	guard := monkey.Patch(GetConfigurationPaths, func() (string, string, string, string) {
		panic("<test-error>")
	})
	defer guard.Unpatch()

	WriteConfigurationOverrideFile(QPepConfigType{})
}

func (s *QPepConfigSuite) TestWriteConfigurationOverrideFile_MarshalError() {
	basedir, _ := os.Executable()
	expectConfDir := filepath.Join(filepath.Dir(basedir), CONFIG_PATH)
	expectUserFile := filepath.Join(expectConfDir, CONFIG_OVERRIDE_FILENAME)

	guard := monkey.Patch(yaml.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("<error>")
	})
	defer guard.Unpatch()

	config := QPepConfigType{}
	config.Merge(&DefaultConfig)

	WriteConfigurationOverrideFile(config)
	_, err := os.Stat(expectUserFile)
	assert.Nil(s.T(), err)
}
