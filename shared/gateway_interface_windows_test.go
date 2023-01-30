//go:build windows

package shared

import (
	"bou.ke/monkey"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"net/url"
	"sort"
	"testing"
)

func TestGatewayConfig(t *testing.T) {
	var q GatewayConfigSuite
	suite.Run(t, &q)
}

type GatewayConfigSuite struct {
	suite.Suite
}

func (s *GatewayConfigSuite) BeforeTest() {
}

func (s *GatewayConfigSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
	usersRegistryKeys = nil
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_False() {
	t := s.T()
	monkey.Patch(runCommand, func(string, ...string) ([]byte, error) {
		return []byte("0x0"), nil
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_False_Error() {
	t := s.T()
	monkey.Patch(runCommand, func(string, ...string) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_True() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if data[len(data)-1] == "ProxyEnable" {
			return []byte("0x1"), nil
		}
		return []byte("127.0.0.1:9090"), nil
	})

	active, url := GetSystemProxyEnabled()
	assert.True(t, active)

	urlExpect, _ := url.Parse("http://127.0.0.1:9090")
	assert.Equal(t, urlExpect, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_True_Error() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if data[len(data)-1] == "ProxyEnable" {
			return []byte("0x1"), nil
		}
		return nil, errors.New("test-error")
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_True_ErrorParse() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if data[len(data)-1] == "ProxyEnable" {
			return []byte("0x1"), nil
		}
		return []byte("%invalid%127.0.0.1:9090"), nil
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestPreloadRegistryKeysForUsers() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		return []byte("SID\nS-1-5-21-4227727717-1300533570-3298936513-500\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-503\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-501\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-1004\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-504\n"), nil
	})

	preloadRegistryKeysForUsers()

	assert.NotNil(t, usersRegistryKeys)
	sort.Strings(usersRegistryKeys)
	l := len(usersRegistryKeys)
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-500\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-503\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-501\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-1004\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-504\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))

	// second time is not reset
	preloadRegistryKeysForUsers()

	assert.NotNil(t, usersRegistryKeys)
	sort.Strings(usersRegistryKeys)
	l = len(usersRegistryKeys)
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-500\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-503\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-501\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-1004\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
	assert.NotEqual(t, l, sort.SearchStrings(usersRegistryKeys, "HKEY_USERS\\S-1-5-21-4227727717-1300533570-3298936513-504\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"))
}

func (s *GatewayConfigSuite) TestPreloadRegistryKeysForUsers_Error() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	assert.Panics(t, func() {
		preloadRegistryKeysForUsers()
	})
}

func (s *GatewayConfigSuite) TestSetSystemProxy_Disabled() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if name == "wmic" {
			return []byte("SID\nS-1-5-21-4227727717-1300533570-3298936513-500\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-503\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-501\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-1004\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-504\n"), nil
		}
		return nil, nil // ignored just don't execute the real command
	})

	SetSystemProxy(false)

	assert.False(t, UsingProxy)
	assert.Nil(t, ProxyAddress)
}

func (s *GatewayConfigSuite) TestSetSystemProxy_Active() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if name == "wmic" {
			return []byte("SID\nS-1-5-21-4227727717-1300533570-3298936513-500\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-503\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-501\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-1004\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-504\n"), nil
		}
		return nil, nil // ignored just don't execute the real command
	})

	SetSystemProxy(true)

	assert.True(t, UsingProxy)

	u, _ := url.Parse(fmt.Sprintf("http://%s:%d", QPepConfig.ListenHost, QPepConfig.ListenPort))
	assert.Equal(t, u, ProxyAddress)
}
