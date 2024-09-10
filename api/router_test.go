package api

import (
	"bou.ke/monkey"
	"context"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/flags"
	"github.com/parvit/qpep/webgui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRouterSuite(t *testing.T) {
	var q RouterSuite
	suite.Run(t, &q)
}

type RouterSuite struct {
	suite.Suite

	ctx      context.Context
	cancel   context.CancelFunc
	finished bool
}

func (s *RouterSuite) BeforeTest(_, testName string) {
	flags.Globals.Client = false

	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)
	configuration.QPepConfig.Server.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.General.APIPort = 9443
	s.finished = false
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var local = true

	switch testName {
	case "TestNewServer":
		s.finished = true
		break
	case "TestRunServer":
		local = false
		fallthrough
	default:
		if strings.Contains(testName, "ClientMode") {
			flags.Globals.Client = true
		}
		go func() {
			RunServer(s.ctx, s.cancel, local)
			s.finished = true
		}()

		<-time.After(1 * time.Second)
		assert.False(s.T(), s.finished)
	}
}

func (s *RouterSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()

	switch testName {
	case "TestNewServer":
		break
	default:
		// stops the launched server
		s.cancel()
		<-time.After(1 * time.Second)
		assert.True(s.T(), s.finished)
	}
}

func (s *RouterSuite) TestNewServer() {
	t := s.T()

	ctx := context.Background()

	router := newRouter()
	assert.NotNil(t, router)

	srv := newServer("127.0.0.1", router, ctx)
	assert.NotNil(t, srv)
}

func (s *RouterSuite) TestRunServer() {
	t := s.T()

	// stops the launched server
	s.cancel()
	<-time.After(1 * time.Second)
	assert.True(t, s.finished)
}

func (s *RouterSuite) TestRunServer_Local() {
	t := s.T()

	// stops the launched server
	s.cancel()
	<-time.After(1 * time.Second)
	assert.True(t, s.finished)
}

func (s *RouterSuite) TestRunServer_ServerModeAPICall_OK() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://localhost:9443"+API_PREFIX_SERVER+API_ECHO_PATH, client)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ServerModeAPICall_Forbidden() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://localhost:9443"+API_PREFIX_CLIENT+API_ECHO_PATH, client)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ServerModeAPICall_NotFound() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://localhost:9443"+API_PREFIX_SERVER+"/notfound", client)
	assert.Nil(t, err)
	assert.NotNil(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ServerModeAPICall_BadRequest() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	req, err := http.NewRequest("GET", "http://localhost:9443"+API_PREFIX_SERVER+API_ECHO_PATH, nil)
	assert.Nil(t, err)
	assert.NotNil(t, req)

	req.Header.Set("User-Agent", runtime.GOOS)
	req.Header.Set("Accept", "text/xml")

	resp, err := client.Do(req)
	defer func() {
		_ = resp.Body.Close()
	}()
	assert.Nil(t, err)
	assert.NotNil(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ClientModeAPICall_OK() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://localhost:9443"+API_PREFIX_CLIENT+API_ECHO_PATH, client)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ClientModeAPICall_Forbidden() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://localhost:9443"+API_PREFIX_SERVER+API_ECHO_PATH, client)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ClientModeAPICall_NotFound() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	resp, err := doAPIRequest("http://localhost:9443"+API_PREFIX_CLIENT+"/notfound", client)
	assert.Nil(t, err)
	assert.NotNil(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ClientModeAPICall_BadRequest() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	req, err := http.NewRequest("GET", "http://localhost:9443"+API_PREFIX_CLIENT+API_ECHO_PATH, nil)
	assert.Nil(t, err)
	assert.NotNil(t, req)

	req.Header.Set("User-Agent", runtime.GOOS)
	req.Header.Set("Accept", "text/xml")

	resp, err := client.Do(req)
	defer func() {
		_ = resp.Body.Close()
	}()
	assert.Nil(t, err)
	assert.NotNil(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func (s *RouterSuite) TestRunServer_ClientMode_GetFile() {
	t := s.T()

	data := s.checkFileTest("http://localhost:9443/favicon.ico", len(webgui.FilesList["favicon.ico"]))

	assertArrayEquals(t, webgui.FilesList["favicon.ico"], data)
}

func (s *RouterSuite) TestRunServer_ClientMode_GetIndexHtml() {
	t := s.T()

	data := s.checkFileTest("http://localhost:9443/", len(webgui.FilesList["index.html"]))

	assertArrayEquals(t, webgui.FilesList["index.html"], data)
}

func (s *RouterSuite) TestRunServer_ClientMode_GetIndex() {
	t := s.T()

	data := s.checkFileTest("http://localhost:9443/index", len(webgui.FilesList["index.html"]))

	assertArrayEquals(t, webgui.FilesList["index.html"], data)
}

func (s *RouterSuite) checkFileTest(address string, expectedSize int) []byte {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	req, err := http.NewRequest("GET", address, nil)
	assert.Nil(t, err)
	assert.NotNil(t, req)
	req.Header.Set("User-Agent", runtime.GOOS)

	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	defer func() {
		_ = resp.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var buffer = make([]byte, expectedSize)
	n, err := resp.Body.Read(buffer)
	assert.Equal(t, io.EOF, err)

	return buffer[:n]
}

func (s *RouterSuite) TestRunServer_ClientMode_GetFile_MethodNotAllowed() {
	t := s.T()
	client := getClientForAPI(&net.TCPAddr{
		IP: net.ParseIP("127.0.0.1"),
	})
	assert.NotNil(t, client)

	req, err := http.NewRequest("POST", "http://localhost:9443/favicon.ico", nil)
	assert.Nil(t, err)
	assert.NotNil(t, req)
	req.Header.Set("User-Agent", runtime.GOOS)

	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	defer func() {
		_ = resp.Body.Close()
	}()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// --- Utils --- //
func assertArrayEquals(t *testing.T, vec_a, vec_b []byte) {
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
