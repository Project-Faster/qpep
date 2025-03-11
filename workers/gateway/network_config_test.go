// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package gateway

import (
	"github.com/Project-Faster/monkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
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
	for _, v := range addrs {
		assert.Greater(t, len(v), 0)
	}
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AlreadyDetected() {
	t := s.T()
	defaultListeningAddress = "127.0.0.1"
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("127.0.0.1", "")
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

func (s *NetworkConfigSuite) TestGetDefaultLanListeningAddresses_AutodetectWithGatewayNotFound() {
	t := s.T()
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"172.168.1.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "192.168.1.1")
	assert.Equal(t, "172.168.1.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

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
