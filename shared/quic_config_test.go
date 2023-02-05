package shared

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestQuicConfig(t *testing.T) {
	var q QuicConfigSuite
	suite.Run(t, &q)
}

type QuicConfigSuite struct{ suite.Suite }

func (s *QuicConfigSuite) BeforeTest() {}

func (s *QuicConfigSuite) AfterTest(_, _ string) {}

func (s *QuicConfigSuite) TestGetQuicConfiguration() {
	cfg := GetQuicConfiguration()
	assert.NotNil(s.T(), cfg)

	assert.Equal(s.T(), int64(40000), cfg.MaxIncomingStreams)
	assert.True(s.T(), cfg.DisablePathMTUDiscovery)

	assert.Nil(s.T(), cfg.Tracer)
}
