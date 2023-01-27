// NOTE: requires flag '-gcflags=-l' to go test to work with monkey patching

package shared

import (
	"bou.ke/monkey"
	"errors"
	"github.com/jackpal/gateway"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
)

func TestGetLanListeningAddresses_Default(t *testing.T) {
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetLanListeningAddresses()
	assertArrayEqualsString(t, []string{"127.0.0.1"}, addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func TestGetDefaultLanListeningAddresses_AlreadyDetected(t *testing.T) {
	defaultListeningAddress = "127.0.0.1"
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("192.0.0.1", "")
	assert.Equal(t, "127.0.0.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func TestGetDefaultLanListeningAddresses_NoAutodetect(t *testing.T) {
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("192.0.0.1", "")
	assert.Equal(t, "192.0.0.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func TestGetDefaultLanListeningAddresses_AutodetectNoGateway(t *testing.T) {
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1"}
	detectedGatewayInterfaces = []int64{1}

	guard := monkey.Patch(gateway.DiscoverInterface, func() (net.IP, error) {
		return net.ParseIP("192.168.1.1"), nil
	})
	defer guard.Restore()

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "")
	assert.Equal(t, "192.168.1.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func TestGetDefaultLanListeningAddresses_AutodetectNoGatewayPanic(t *testing.T) {
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

func TestGetDefaultLanListeningAddresses_AutodetectWithGatewayNotFound(t *testing.T) {
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"172.168.1.1"}
	detectedGatewayInterfaces = []int64{1}

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "192.168.1.1")
	assert.Equal(t, "172.168.1.1", addrs)
	assertArrayEqualsInt64(t, []int64{1}, interfaces)
}

func TestGetDefaultLanListeningAddresses_AutodetectWithGatewayFound(t *testing.T) {
	defaultListeningAddress = ""
	detectedGatewayAddresses = []string{"127.0.0.1", "192.0.1.1", "192.168.0.1", "192.168.1.100"}
	detectedGatewayInterfaces = []int64{1, 2, 3, 4}

	addrs, interfaces := GetDefaultLanListeningAddress("0.0.0.0", "192.168.1.1")
	assert.Equal(t, "192.168.1.100", addrs)
	assertArrayEqualsInt64(t, []int64{1, 2, 3, 4}, interfaces)
}

func TestGetDefaultLanListeningAddresses_AutodetectWithGatewayFoundExact(t *testing.T) {
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
