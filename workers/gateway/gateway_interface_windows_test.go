//go:build windows

package gateway

import (
	"bou.ke/monkey"
	stderr "errors"
	"fmt"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/errors"
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

func (s *GatewayConfigSuite) BeforeTest(_, _ string) {
	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)

	configuration.QPepConfig.Client.GatewayHost = "127.0.0.1"
	configuration.QPepConfig.Client.GatewayPort = 9443
	configuration.QPepConfig.Client.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.Client.LocalListenPort = 9090
}

func (s *GatewayConfigSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
	usersRegistryKeys = nil
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_False() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(string, ...string) ([]byte, error, int) {
		return []byte("0x0"), nil, 0
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_False_Error() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(string, ...string) ([]byte, error, int) {
		return nil, stderr.New("test-error"), 1
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_True() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if data[len(data)-1] == "ProxyEnable" {
			return []byte("0x1"), nil, 0
		}
		return []byte("127.0.0.1:9090"), nil, 0
	})

	active, url := GetSystemProxyEnabled()
	assert.True(t, active)

	urlExpect, _ := url.Parse("http://127.0.0.1:9090")
	assert.Equal(t, urlExpect, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_True_Error() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if data[len(data)-1] == "ProxyEnable" {
			return []byte("0x1"), nil, 0
		}
		return nil, stderr.New("test-error"), 1
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestGetSystemProxyEnabled_True_ErrorParse() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if data[len(data)-1] == "ProxyEnable" {
			return []byte("0x1"), nil, 0
		}
		return []byte("%invalid%127.0.0.1:9090"), nil, 0
	})

	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func (s *GatewayConfigSuite) TestPreloadRegistryKeysForUsers() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		return []byte("SID\nS-1-5-21-4227727717-1300533570-3298936513-500\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-503\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-501\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-1004\n" +
			"S-1-5-21-4227727717-1300533570-3298936513-504\n"), nil, 0
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
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		return nil, stderr.New("test-error"), 1
	})

	assert.Panics(t, func() {
		preloadRegistryKeysForUsers()
	})
}

func (s *GatewayConfigSuite) TestSetSystemProxy_Disabled() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if name == "wmic" {
			return []byte("SID\nS-1-5-21-4227727717-1300533570-3298936513-500\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-503\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-501\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-1004\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-504\n"), nil, 0
		}
		return nil, nil, 0 // ignored just don't execute the real command
	})

	SetSystemProxy(false)

	assert.False(t, UsingProxy)
	assert.Nil(t, ProxyAddress)
}

func (s *GatewayConfigSuite) TestSetSystemProxy_Active() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if name == "wmic" {
			return []byte("SID\nS-1-5-21-4227727717-1300533570-3298936513-500\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-503\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-501\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-1004\n" +
				"S-1-5-21-4227727717-1300533570-3298936513-504\n"), nil, 0
		}
		return nil, nil, 0 // ignored just don't execute the real command
	})

	SetSystemProxy(true)

	assert.True(t, UsingProxy)

	u, _ := url.Parse(fmt.Sprintf("http://%s:%d",
		configuration.QPepConfig.Client.LocalListeningAddress,
		configuration.QPepConfig.Client.LocalListenPort))
	assert.Equal(t, u, ProxyAddress)
}

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces_ErrorRoute() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if data[len(data)-1] == "route" {
			return nil, stderr.New("test-error"), 1
		}
		return nil, nil, 0 // ignored just don't execute the real command
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, errors.ErrFailedGatewayDetect, err)
}

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces_ErrorRouteEmpty() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if data[len(data)-1] == "route" {
			return []byte(``), nil, 0
		}
		return nil, nil, 0 // ignored just don't execute the real command
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, errors.ErrFailedGatewayDetect, err)
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
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		if data[len(data)-1] == "route" {
			return cmdTestDataRoute, nil, 0
		}
		return nil, stderr.New("test-error"), 1
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, errors.ErrFailedGatewayDetect, err)
}

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces_ErrorConfig() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		switch data[len(data)-1] {
		case "route":
			return cmdTestDataRoute, nil, 0
		case "interface":
			return cmdTestDataInterfaces, nil, 0
		}
		return nil, stderr.New("test-error"), 1
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, interfacesList)
	assert.Nil(t, addressList)
	assert.Equal(t, errors.ErrFailedGatewayDetect, err)
}

func (s *GatewayConfigSuite) TestGetRouteGatewayInterfaces() {
	t := s.T()
	monkey.Patch(shared.RunCommand, func(name string, data ...string) ([]byte, error, int) {
		switch data[len(data)-1] {
		case "route":
			return cmdTestDataRoute, nil, 0
		case "interface":
			return cmdTestDataInterfaces, nil, 0
		case "config":
			return cmdTestDataConfig, nil, 0
		}
		return nil, stderr.New("test-error"), 1
	})

	interfacesList, addressList, err := getRouteGatewayInterfaces()
	assert.Nil(t, err)
	assert.Equal(t, int64(18), interfacesList[0])
	assert.Equal(t, "192.168.1.46", addressList[0])
}
