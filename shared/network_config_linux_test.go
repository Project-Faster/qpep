//go:build linux

// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package shared

import (
	"github.com/stretchr/testify/assert"
)

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AutodetectWithGatewayFound() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1", "192.0.1.1", "192.168.0.1", "192.168.1.100"}
	detectedGatewayInterfaces = []int64{1, 2, 3, 4}

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "192.168.1.1")
	assert.Equal(t, "192.168.1.100", addrs)
	assertArrayEqualsInt64(t, []int64{1, 2, 3, 4}, interfaces)
}

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AutodetectWithGatewayFoundExact() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1", "192.168.1.1", "192.168.0.1", "192.168.1.100"}
	detectedGatewayInterfaces = []int64{1, 2, 3, 4}

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "192.168.1.1")
	assert.Equal(t, "192.168.1.1", addrs)
	assertArrayEqualsInt64(t, []int64{1, 2, 3, 4}, interfaces)
}
