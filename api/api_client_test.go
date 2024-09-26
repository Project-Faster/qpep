package api

import (
	"bou.ke/monkey"
	"context"
	"encoding/json"
	"errors"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/flags"
	"github.com/parvit/qpep/shared/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestAPIClientSuite(t *testing.T) {
	var q APIClientSuite
	suite.Run(t, &q)
}

type APIClientSuite struct {
	suite.Suite

	ctx      context.Context
	cancel   context.CancelFunc
	finished bool
}

func (s *APIClientSuite) BeforeTest(_, testName string) {
	Statistics.Reset()

	flags.Globals.Client = false
	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)
	configuration.QPepConfig.General.Verbose = false
	configuration.QPepConfig.General.APIPort = 9443
	configuration.QPepConfig.Server.LocalListeningAddress = "127.0.0.1"
	s.finished = false
	s.ctx, s.cancel = context.WithCancel(context.Background())

	go func() {
		RunServer(s.ctx, s.cancel, true)
		s.finished = true
	}()

	<-time.After(1 * time.Second)
	assert.False(s.T(), s.finished)
}

func (s *APIClientSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()

	// stops the launched server
	s.cancel()
	<-time.After(1 * time.Second)
	assert.True(s.T(), s.finished)
}

func (s *APIClientSuite) TestGetClientForAPI() {
	t := s.T()

	client := getClientForAPI(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})

	assert.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)

	assert.NotNil(t, client.Transport)
}

func (s *APIClientSuite) TestDoAPIRequest() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://127.0.0.1:9443"+API_PREFIX_SERVER+API_ECHO_PATH, client)
	assert.NotNil(t, resp)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body := make([]byte, 1024)
	n, err := resp.Body.Read(body)
	assert.Nil(t, err)

	t.Logf("data: %v\n", string(body[:n]))

	assert.NotEqual(t, 0, n)
}

func (s *APIClientSuite) TestDoAPIRequest_ErrNewRequest() {
	t := s.T()

	monkey.Patch(http.NewRequest, func(string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("test-error")
	})

	client := getClientForAPI(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://127.0.0.1:9443"+API_PREFIX_SERVER+API_ECHO_PATH, client)
	assert.Nil(t, resp)
	assert.NotNil(t, err)
}

func (s *APIClientSuite) TestDoAPIRequest_ErrDoRequest() {
	t := s.T()

	client := getClientForAPI(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://127.0.0.1:7777"+API_PREFIX_SERVER+API_ECHO_PATH, client)
	assert.Nil(t, resp)
	assert.NotNil(t, err)
}

func (s *APIClientSuite) TestRequestEcho() {
	t := s.T()
	assert.Equal(t, "", Statistics.GetState(INFO_OTHER_VERSION))

	resp := RequestEcho("", "", 9443, true)
	assert.NotNil(t, resp)

	assert.Equal(t, "127.0.0.1", resp.Address)
	assert.NotEqual(t, 0, resp.Port)
	assert.Equal(t, version.Version(), resp.ServerVersion)

	assert.Equal(t, version.Version(), Statistics.GetState(INFO_OTHER_VERSION))
}

func (s *APIClientSuite) TestRequestEcho_Client() {
	t := s.T()
	resp := RequestEcho("", "", 9443, false)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestEcho_FailLocalAddress() {
	t := s.T()
	resp := RequestEcho("fail", "", 9443, true)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestEcho_FailAddress() {
	t := s.T()
	resp := RequestEcho("", "fail", 9443, true)
	assert.Nil(t, resp)

	resp = RequestEcho("", "", 99999, true)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestEcho_FailJSON() {
	t := s.T()

	monkey.Patch(json.Unmarshal, func([]byte, interface{}) error {
		return errors.New("test-error")
	})

	resp := RequestEcho("", "", 9443, true)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestStatus() {
	t := s.T()

	resp := RequestStatus("", "", 9443, "127.0.0.1", true)
	assert.NotNil(t, resp)

	assert.NotEqual(t, 0, resp.ConnectionCounter)
	assert.NotEqual(t, "", resp.LastCheck)
}

func (s *APIClientSuite) TestRequestStatus_Client() {
	t := s.T()
	resp := RequestStatus("", "", 9443, "127.0.0.1", false)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestStatus_FailLocalAddress() {
	t := s.T()
	resp := RequestStatus("fail", "", 9443, "127.0.0.1", true)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestStatus_FailAddress() {
	t := s.T()
	resp := RequestStatus("", "fail", 9443, "127.0.0.1", true)
	assert.Nil(t, resp)

	resp = RequestStatus("", "", 99999, "", true)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestStatus_FailJSON() {
	t := s.T()

	monkey.Patch(json.Unmarshal, func([]byte, interface{}) error {
		return errors.New("test-error")
	})

	resp := RequestStatus("", "", 9443, "127.0.0.1", true)
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestStatistics() {
	t := s.T()

	resp := RequestStatistics("", "", 9443, "127.0.0.1")
	assert.NotNil(t, resp)

	assert.NotEqual(t, 0, len(resp.Data))
}

func (s *APIClientSuite) TestRequestStatistics_FailLocalAddress() {
	t := s.T()
	resp := RequestStatistics("fail", "", 9443, "127.0.0.1")
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestStatistics_FailAddress() {
	t := s.T()
	resp := RequestStatistics("", "fail", 9443, "127.0.0.1")
	assert.Nil(t, resp)

	resp = RequestStatistics("", "", 99999, "")
	assert.Nil(t, resp)
}

func (s *APIClientSuite) TestRequestStatistics_FailJSON() {
	t := s.T()

	monkey.Patch(json.Unmarshal, func([]byte, interface{}) error {
		return errors.New("test-error")
	})

	resp := RequestStatistics("", "", 9443, "127.0.0.1")
	assert.Nil(t, resp)
}
