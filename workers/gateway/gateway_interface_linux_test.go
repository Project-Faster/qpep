//go:build !windows

package gateway

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestGatewayConfig(t *testing.T) {
	var q GatewayConfigSuite
	suite.Run(t, &q)
}

type GatewayConfigSuite struct{ suite.Suite }

func (s *GatewayConfigSuite) BeforeTest() {
}

func (s *GatewayConfigSuite) AfterTest() {
}

func TestGetRouteGatewayInterfaces(t *testing.T) {
	v1, v2, v3 := getRouteGatewayInterfaces()
	assert.Len(t, v1, 0)
	assert.Len(t, v2, 1)
	assert.Nil(t, v3)
}

func TestGetSystemProxyEnabled(t *testing.T) {
	active, url := GetSystemProxyEnabled()
	assert.False(t, active)
	assert.Nil(t, url)
}

func TestSetSystemProxy(t *testing.T) {
	// no actual effect currently
	SetSystemProxy(true)
	SetSystemProxy(false)
}
