//go:build !arm64

// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package gateway

import (
	"errors"
	"github.com/Project-Faster/monkey"
	"github.com/jackpal/gateway"
	"github.com/stretchr/testify/assert"
	"net"
)

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AutodetectNoGateway() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	monkey.Patch(gateway.DiscoverInterface, func() (net.IP, error) {
		return net.ParseIP("192.168.1.1"), nil
	})

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "")
	assert.Equal(t, "192.168.1.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AutodetectNoGatewayPanic() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	guard := monkey.Patch(gateway.DiscoverInterface, func() (net.IP, error) {
		return nil, errors.New("<test-error>")
	})
	defer guard.Restore()

	assert.Panics(t, func() {
		GetDefaultLanListeningAddress("0.0.0.0", "")
	})
}
