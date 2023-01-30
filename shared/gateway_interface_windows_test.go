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

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces_ErrorRoute() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if data[len(data)-1] == "route" {
			return nil, errors.New("test-error")
		}
		return nil, nil // ignored just don't execute the real command
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, ErrFailedGatewayDetect, err)
}

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces_ErrorRouteEmpty() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if data[len(data)-1] == "route" {
			return []byte(``), nil
		}
		return nil, nil // ignored just don't execute the real command
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, ErrFailedGatewayDetect, err)
}

var cmdTestDataRoute = []byte(`
Tipo pubblicazione      Prefisso met.                  Gateway idx/Nome interfaccia
-------  --------  ---  ------------------------  ---  ------------------------
No       Manuale   0    0.0.0.0/0                  18  192.168.1.1
No       Manuale   0    0.0.0.0/0                  20  192.168.1.1
No       Sistema   256  127.0.0.0/8                 1  Loopback Pseudo-Interface 1
No       Sistema   256  192.168.1.0/24             18  Wi-Fi
No       Sistema   256  192.168.1.0/24             20  Ethernet
`)

var cmdTestDataInterfaces = []byte(`
Idx     Met.         MTU          Stato                Nome
---  ----------  ----------  ------------  ---------------------------
  1          75  4294967295  connected     Loopback Pseudo-Interface 1
 18          35        1300  connected     Ethernet
 16          50        1300  disconnected  Wi-Fi
`)

var cmdTestDataConfig = []byte(`
Configurazione per l'interfaccia "Ethernet"                                           
    DHCP abilitato:                         Sì                                        
    Indirizzo IP:                           192.168.1.46                              
    Prefisso subnet:                        192.168.1.0/24 (maschera 255.255.255.0)   
    Gateway predefinito:                      192.168.1.1                             
    Metrica gateway:                          0                                       
    MetricaInterfaccia:                      35                                       
    Server DNS configurati statisticamente:    192.168.1.1                            
                                          192.168.1.254                               
    Registra con suffisso:           Solo primario                                    
    Server WINS configurati tramite DHCP:  nessuno                                    
                                                                                      
Configurazione per l'interfaccia "Wi-Fi"                                              
    DHCP abilitato:                         Sì                                        
    Indirizzo IP:                           192.168.1.63                              
    Prefisso subnet:                        192.168.1.0/24 (maschera 255.255.255.0)   
    Gateway predefinito:                      192.168.1.1                             
    Metrica gateway:                          0                                       
    MetricaInterfaccia:                      50                                       
    Server DNS configurati tramite DHCP:  192.168.1.1                                 
    Registra con suffisso:           Solo primario                                    
    Server WINS configurati tramite DHCP:  nessuno                                    
                                                                                      
Configurazione per l'interfaccia "Loopback Pseudo-Interface 1"                        
    DHCP abilitato:                         No                                        
    Indirizzo IP:                           127.0.0.1                                 
    Prefisso subnet:                        127.0.0.0/8 (maschera 255.0.0.0)          
    MetricaInterfaccia:                      75                                       
    Server DNS configurati statisticamente:    nessuno                                
    Registra con suffisso:           Solo primario                                    
    Server WINS configurati statisticamente:    nessuno                               
                                                                                      
                                                                                      `)

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces_ErrorInterface() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		if data[len(data)-1] == "route" {
			return cmdTestDataRoute, nil
		}
		return nil, errors.New("test-error")
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, ErrFailedGatewayDetect, err)
}

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces_ErrorConfig() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		switch data[len(data)-1] {
		case "route":
			return cmdTestDataRoute, nil
		case "interface":
			return cmdTestDataInterfaces, nil
		}
		return nil, errors.New("test-error")
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, ErrFailedGatewayDetect, err)
}

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces() {
	t := s.T()
	monkey.Patch(runCommand, func(name string, data ...string) ([]byte, error) {
		switch data[len(data)-1] {
		case "route":
			return cmdTestDataRoute, nil
		case "interface":
			return cmdTestDataInterfaces, nil
		case "config":
			return cmdTestDataConfig, nil
		}
		return nil, errors.New("test-error")
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, err)
	assertArrayEqualsInt64(t, []int64{18}, interfacesList)
	assertArrayEqualsString(t, []string{"192.168.1.46"}, addressList)
}
