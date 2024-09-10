package backend

import (
	"bou.ke/monkey"
	"context"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestBackendFactorySuite(t *testing.T) {
	var q BackendFactorySuite
	suite.Run(t, &q)
}

type BackendFactorySuite struct {
	suite.Suite
}

func (s *BackendFactorySuite) BeforeTest(_, testName string) {
}

func (s *BackendFactorySuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
}

func (s *BackendFactorySuite) TestRegister() {
	bcRegister = nil
	bcList = nil

	Register("test", &testBackend{})
}

func (s *BackendFactorySuite) TestRegisterAddedAlready() {
	bcRegister = nil
	bcList = nil

	Register("test", &testBackend{})
	Register("test", &testBackend{})
}

func (s *BackendFactorySuite) TestRegisterGet() {
	bcRegister = nil
	bcList = nil
	bcDefaultBackend = "test"

	var defaultBackend = &testBackend{}
	Register("test", defaultBackend)

	var backend, found = Get("test")
	s.True(found)
	s.NotNil(backend)

	backend, found = Get("test-not-present")
	s.False(found)
	s.Equal(defaultBackend, backend)
}

func (s *BackendFactorySuite) TestRegisterList() {
	bcList = []string{"test"}

	var ret = List()
	s.NotNil(ret)
	s.Equal("test", ret[0])
}

// ----- Support ----- //

type testBackend struct{}

func (t testBackend) Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	return nil, nil
}
func (t testBackend) Listen(ctx context.Context, address string, port int, serverCertPath string, serverKeyPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	return nil, nil
}
func (t testBackend) Close() error {
	return nil
}
