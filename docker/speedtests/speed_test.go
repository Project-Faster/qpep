package speedtests

import (
	"flag"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"
)

var targetURL = flag.String("target_url", "", "url to download")
var connections = flag.Int("connections_num", 1, "simultaneous tcp connections to make to the server")

func TestSpeedTestsConfigSuite(t *testing.T) {
	t.Log(*targetURL)
	t.Log(*connections)

	assert.True(t, *connections > 0)
	assert.True(t, len(*targetURL) > 0)

	var q SpeedTestsConfigSuite
	suite.Run(t, &q)
}

type SpeedTestsConfigSuite struct {
	suite.Suite
}

func (s *SpeedTestsConfigSuite) TestRun() {
	shared.GetSystemProxyEnabled()

	wg := &sync.WaitGroup{}
	wg.Add(*connections)

	index := 0

	go func(id int) {
		defer func() {
			s.T().Logf("Executor #%d done\n", id)
			wg.Done()
		}()
		s.T().Logf("Starting executor #%d\n", id)

		client := getClientForAPI(nil)
		assert.NotNil(s.T(), client)
		assert.NotNil(s.T(), targetURL)

		_, err := client.Get(*targetURL)
		assert.Nil(s.T(), err)
	}(index)

	wg.Wait()
}

func getClientForAPI(localAddr net.Addr) *http.Client {
	dialer := &net.Dialer{
		LocalAddr: localAddr,
		Timeout:   2 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				shared.UsingProxy, shared.ProxyAddress = shared.GetSystemProxyEnabled()
				logger.Info("API Proxy: %v %v\n", shared.UsingProxy, shared.ProxyAddress)
				if shared.UsingProxy {
					return shared.ProxyAddress, nil
				}
				return nil, nil
			},
			DialContext:     dialer.DialContext,
			MaxIdleConns:    1,
			IdleConnTimeout: 10 * time.Second,
			//TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}