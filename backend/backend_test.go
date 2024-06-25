package backend

import (
	"bou.ke/monkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestBackendSuite(t *testing.T) {
	var q BackendSuite
	suite.Run(t, &q)
}

type BackendSuite struct {
	suite.Suite
}

func (s *BackendSuite) BeforeTest(_, testName string) {
}

func (s *BackendSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
}

func (s *BackendSuite) TestGenerateTLSConfig() {
	config := GenerateTLSConfig("cert.pem", "key.pem")
	assert.NotNil(s.T(), config)
}
