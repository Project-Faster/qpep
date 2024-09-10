// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package gateway

import (
	"bou.ke/monkey"
	"errors"
	"github.com/jackpal/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"net"
	"testing"
)

func TestNetworkConfigSuite(t *testing.T) {
	var q NetworkConfigSuite
	suite.Run(t, &q)
}

type NetworkConfigSuite struct {
	suite.Suite
}

func (s *NetworkConfigSuite) BeforeTest(_, testName string) {
	defaultListeningAddress = ""
	detectedGatewayInterfaces = nil
	detectedGatewayAddresses = nil
}

func (s *NetworkConfigSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
}

func (s *NetworkConfigSuite) TestGetLanListeningAddresses_Default() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetLanListeningAddresses()
	assertArrayEqualsString(t, []string{"127.0.0.1"}, addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AlreadyDetected() {
	t := s.T()
	defaultListeningAddress = "127.0.0.1"
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("192.0.0.1", "")
	assert.Equal(t, "127.0.0.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_NoAutodetect() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("192.0.0.1", "")
	assert.Equal(t, "192.0.0.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

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

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AutodetectWithGatewayNotFound() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"172.168.1.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "192.168.1.1")
	assert.Equal(t, "172.168.1.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

// Utils
func assertArrayEqualsString(t *testing.T, vec_a, vec_b []string) {
	assert.Equal(t, len(vec_a), len(vec_b))
	if t.Failed() {
		t.Logf("a: %v, b: %v\n", vec_a, vec_b)
		return
	}

	for i := 0; i < len(vec_a); i++ {
		assert.Equal(t, vec_a[i], vec_b[i])
		if t.Failed() {
			t.Logf("a: %v, b: %v\n", vec_a, vec_b)
			return
		}
	}
}

func assertArrayEqualsInt64(t *testing.T, vec_a, vec_b []int64) {
	assert.Equal(t, len(vec_a), len(vec_b))
	if t.Failed() {
		t.Logf("a: %v, b: %v\n", vec_a, vec_b)
		return
	}

	for i := 0; i < len(vec_a); i++ {
		assert.Equal(t, vec_a[i], vec_b[i])
		if t.Failed() {
			t.Logf("a: %v, b: %v\n", vec_a, vec_b)
			return
		}
	}
}
