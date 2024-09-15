//go:build !arm64

package client

import (
	"context"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/windivert"
	"github.com/stretchr/testify/assert"
	"net"
	"net/url"
	"runtime"
	"sync"
	"time"
)

func (s *ClientSuite) TestRunClient() {
	var calledNewProxy = false
	monkey.Patch(NewClientProxyListener, func(string, *net.TCPAddr) (net.Listener, error) {
		calledNewProxy = true
		return nil, nil
	})
	var calledListen = false
	monkey.Patch(listenTCPConn, func(wg *sync.WaitGroup) {
		defer wg.Done()
		<-time.After(1 * time.Second)
		calledListen = true
	})
	var calledHandle = false
	monkey.Patch(handleServices, func(_ context.Context, _ context.CancelFunc, wg *sync.WaitGroup) {
		defer wg.Done()
		<-time.After(1 * time.Second)
		calledHandle = true
	})

	ctx, cancel := context.WithCancel(context.Background())
	RunClient(ctx, cancel)

	assert.True(s.T(), calledNewProxy)
	assert.True(s.T(), calledListen)
	assert.True(s.T(), calledHandle)

	assert.NotNil(s.T(), <-ctx.Done())
}

func (s *ClientSuite) TestHandleServices() {
	proxyListener, _ = net.Listen("tcp", "127.0.0.1:9090")
	defer func() {
		if proxyListener != nil {
			_ = proxyListener.Close()
		}
	}()

	wg2 := &sync.WaitGroup{}
	wg2.Add(3)

	var calledInitialCheck = false
	monkey.Patch(initialCheckConnection, func() {
		if !calledInitialCheck {
			calledInitialCheck = true
			wg2.Done()
		}
	})
	var calledGatewayCheck = false
	monkey.Patch(gatewayStatusCheck, func(string, string, int) (bool, *api.EchoResponse) {
		if !calledGatewayCheck {
			calledGatewayCheck = true
			wg2.Done()
		}
		return true, &api.EchoResponse{
			Address:       "172.20.50.150",
			Port:          54635,
			ServerVersion: "0.1.0",
		}
	})
	var calledStatsUpdate = false
	monkey.Patch(clientStatisticsUpdate, func(_ string, _ string, _ int, pubAddress string) bool {
		assert.Equal(s.T(), "172.20.50.150", pubAddress)
		if !calledStatsUpdate {
			calledStatsUpdate = true
			wg2.Done()
		}
		return true
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	ch := make(chan struct{})

	go func() {
		wg2.Wait()
		cancel()
		wg.Wait()

		ch <- struct{}{}
	}()

	select {
	case <-time.After(10 * time.Second):
		s.T().Logf("Test Timed out waiting for routines to finish")
		s.T().FailNow()
		return
	case <-ch:
		break
	}

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
	assert.True(s.T(), calledStatsUpdate)
}

func (s *ClientSuite) TestHandleServices_PanicCheck() {
	var calledInitialCheck = false
	monkey.Patch(initialCheckConnection, func() {
		calledInitialCheck = true
		panic("test-error")
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	cancel()
	wg.Wait()

	assert.True(s.T(), calledInitialCheck)
}

func (s *ClientSuite) TestHandleServices_FailGateway() {
	wg2 := &sync.WaitGroup{}
	wg2.Add(3)
	var calledInitialCheck = false
	monkey.Patch(initialCheckConnection, func() {
		if !calledInitialCheck {
			calledInitialCheck = true
			wg2.Done()
		}
	})
	var calledGatewayCheck = false
	monkey.Patch(gatewayStatusCheck, func(string, string, int) (bool, *api.EchoResponse) {
		if !calledGatewayCheck {
			calledGatewayCheck = true
			wg2.Done()
		}
		return false, nil
	})
	var calledFailedConnectionFirst = false
	var calledFailedConnectionSecond = false
	monkey.Patch(failedCheckConnection, func() bool {
		if !calledFailedConnectionFirst {
			calledFailedConnectionFirst = true
			return false
		}
		if !calledFailedConnectionSecond {
			calledFailedConnectionSecond = true
			wg2.Done()
		}
		return true
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	ch := make(chan struct{})

	go func() {
		wg2.Wait()
		cancel()
		wg.Wait()

		ch <- struct{}{}
	}()

	select {
	case <-time.After(10 * time.Second):
		s.T().Logf("Test Timed out waiting for routines to finish")
		s.T().FailNow()
		return
	case <-ch:
		break
	}

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
	assert.True(s.T(), calledFailedConnectionFirst)
	assert.True(s.T(), calledFailedConnectionSecond)
}

func (s *ClientSuite) TestHandleServices_FailStatistics() {
	wg2 := &sync.WaitGroup{}
	wg2.Add(4)
	var calledInitialCheck = false
	monkey.Patch(initialCheckConnection, func() {
		if !calledInitialCheck {
			calledInitialCheck = true
			wg2.Done()
		}
	})
	var calledGatewayCheck = false
	monkey.Patch(gatewayStatusCheck, func(string, string, int) (bool, *api.EchoResponse) {
		if !calledGatewayCheck {
			calledGatewayCheck = true
			wg2.Done()
		}
		return true, &api.EchoResponse{
			Address:       "172.20.50.150",
			Port:          54635,
			ServerVersion: "0.1.0",
		}
	})
	var calledStatsUpdate = false
	monkey.Patch(clientStatisticsUpdate, func(_ string, _ string, _ int, pubAddress string) bool {
		assert.Equal(s.T(), "172.20.50.150", pubAddress)
		if !calledStatsUpdate {
			calledStatsUpdate = true
			wg2.Done()
		}
		return false
	})
	var calledFailedConnectionFirst = false
	var calledFailedConnectionSecond = false
	monkey.Patch(failedCheckConnection, func() bool {
		if !calledFailedConnectionFirst {
			calledFailedConnectionFirst = true
			return false
		}
		if !calledFailedConnectionSecond {
			calledFailedConnectionSecond = true
			wg2.Done()
		}
		return true
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	ch := make(chan struct{})

	go func() {
		wg2.Wait()
		cancel()
		wg.Wait()

		ch <- struct{}{}
	}()

	select {
	case <-time.After(10 * time.Second):
		s.T().Logf("Test Timed out waiting for routines to finish")
		s.T().FailNow()
		return
	case <-ch:
		break
	}

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
	assert.True(s.T(), calledStatsUpdate)
	assert.True(s.T(), calledFailedConnectionFirst)
	assert.True(s.T(), calledFailedConnectionSecond)
}

func (s *ClientSuite) TestInitProxy() {
	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		shared.UsingProxy = true
		shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), shared.UsingProxy)
	initProxy()
	assert.True(s.T(), shared.UsingProxy)

	assert.NotNil(s.T(), shared.ProxyAddress)
}

func (s *ClientSuite) TestStopProxy() {
	shared.UsingProxy = true
	shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")

	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		shared.UsingProxy = false
		shared.ProxyAddress = nil
	})

	assert.True(s.T(), shared.UsingProxy)
	stopProxy()
	assert.False(s.T(), shared.UsingProxy)
	assert.False(s.T(), redirected)

	assert.Nil(s.T(), shared.ProxyAddress)
}

func (s *ClientSuite) TestInitDiverter() {
	if runtime.GOOS != "windows" {
		assert.False(s.T(), initDiverter())
		return
	}
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		return windivert.DIVERT_OK
	})
	assert.True(s.T(), initDiverter())
}

func (s *ClientSuite) TestInitDiverter_Fail() {
	if runtime.GOOS != "windows" {
		assert.False(s.T(), initDiverter())
		return
	}
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		return windivert.DIVERT_ERROR_ALREADY_INIT
	})
	assert.False(s.T(), initDiverter())
}

func (s *ClientSuite) TestStopDiverter() {
	if runtime.GOOS != "windows" {
		assert.False(s.T(), initDiverter())
		return
	}
	monkey.Patch(windivert.CloseWinDivertEngine, func() int {
		return windivert.DIVERT_OK
	})
	stopDiverter()
	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestInitialCheckConnection() {
	shared.QPepConfig.PreferProxy = false
	validateConfiguration()

	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		shared.UsingProxy = true
		shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), shared.UsingProxy)
	initialCheckConnection()
	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)

	initialCheckConnection()
	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)
}

func (s *ClientSuite) TestGatewayStatusCheck() {
	monkey.Patch(api.RequestEcho, func(string, string, int, bool) *api.EchoResponse {
		return &api.EchoResponse{
			Address:       "127.0.0.1",
			Port:          9090,
			ServerVersion: "0.1.0",
		}
	})

	ok, resp := gatewayStatusCheck("127.0.0.1", "127.0.0.1", 8080)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "127.0.0.1", resp.Address)
	assert.Equal(s.T(), int64(9090), resp.Port)
	assert.Equal(s.T(), "0.1.0", resp.ServerVersion)
}

func (s *ClientSuite) TestGatewayStatusCheck_Fail() {
	monkey.Patch(api.RequestEcho, func(string, string, int, bool) *api.EchoResponse {
		return nil
	})

	ok, resp := gatewayStatusCheck("127.0.0.1", "127.0.0.1", 8080)
	assert.False(s.T(), ok)
	assert.Nil(s.T(), resp)
}

func (s *ClientSuite) TestClientStatisticsUpdate() {
	monkey.Patch(api.RequestStatistics, func(string, string, int, string) *api.StatsInfoResponse {
		return &api.StatsInfoResponse{
			Data: []api.StatsInfo{
				{
					ID:        1,
					Attribute: "Current Connections",
					Value:     "30",
					Name:      api.PERF_CONN,
				},
				{
					ID:        2,
					Attribute: "Current Upload Speed",
					Value:     "0.99",
					Name:      api.PERF_UP_SPEED,
				},
				{
					ID:        3,
					Attribute: "Current Download Speed",
					Value:     "1.00",
					Name:      api.PERF_DW_SPEED,
				},
				{
					ID:        4,
					Attribute: "Total Uploaded Bytes",
					Value:     "100.00",
					Name:      api.PERF_UP_TOTAL,
				},
				{
					ID:        5,
					Attribute: "Total Downloaded Bytes",
					Value:     "10.00",
					Name:      api.PERF_DW_TOTAL,
				},
			},
		}
	})

	ok := clientStatisticsUpdate("127.0.0.1", "127.0.0.1", 8080, "172.30.54.250")
	assert.True(s.T(), ok)

	assert.Equal(s.T(), 30.0, api.Statistics.GetCounter(api.PERF_CONN))
	assert.Equal(s.T(), 0.99, api.Statistics.GetCounter(api.PERF_UP_SPEED))
	assert.Equal(s.T(), 1.0, api.Statistics.GetCounter(api.PERF_DW_SPEED))
	assert.Equal(s.T(), 100.0, api.Statistics.GetCounter(api.PERF_UP_TOTAL))
	assert.Equal(s.T(), 10.0, api.Statistics.GetCounter(api.PERF_DW_TOTAL))
}

func (s *ClientSuite) TestClientStatisticsUpdate_Fail() {
	monkey.Patch(api.RequestStatistics, func(string, string, int, string) *api.StatsInfoResponse {
		return nil
	})

	ok := clientStatisticsUpdate("127.0.0.1", "127.0.0.1", 8080, "172.30.54.250")
	assert.False(s.T(), ok)
}
