//go:build !arm64

package client

import (
	"context"
	"fmt"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/workers/gateway"
	"github.com/stretchr/testify/assert"
	"net"
	"net/url"
	"reflect"
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
	proxyListener, _ = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.testLocalListenPort))
	defer func() {
		if proxyListener != nil {
			_ = proxyListener.Close()
		}
	}()

	wg2 := &sync.WaitGroup{}
	wg2.Add(2)

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
	case <-time.After((GATEWAY_CHECK_WAIT * 3) + (1 * time.Second)):
		s.T().Logf("Test Timed out waiting for routines to finish")
		s.T().FailNow()
		return
	case <-ch:
		break
	}

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
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
	case <-time.After((GATEWAY_CHECK_WAIT * 3) + (1 * time.Second)):
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

func (s *ClientSuite) TestInitProxy() {
	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		gateway.UsingProxy = true
		gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), gateway.UsingProxy)
	initProxy()
	assert.True(s.T(), gateway.UsingProxy)

	assert.NotNil(s.T(), gateway.ProxyAddress)
}

func (s *ClientSuite) TestStopProxy() {
	gateway.UsingProxy = true
	gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		gateway.UsingProxy = false
		gateway.ProxyAddress = nil
	})

	assert.True(s.T(), gateway.UsingProxy)
	stopProxy()
	assert.False(s.T(), gateway.UsingProxy)
	assert.False(s.T(), redirected)

	assert.Nil(s.T(), gateway.ProxyAddress)
}

func (s *ClientSuite) TestInitDiverter() {
	redirected = false
	monkey.Patch(gateway.SetConnectionDiverter, func(active bool, _ string, _ string, _ int, _ int, _ int, _ int64, _ []int) bool {
		assert.True(s.T(), active)
		redirected = true
		return true
	})
	assert.True(s.T(), initDiverter())
	assert.True(s.T(), redirected)
}

func (s *ClientSuite) TestInitDiverter_Fail() {
	redirected = true
	monkey.Patch(gateway.SetConnectionDiverter, func(active bool, _ string, _ string, _ int, _ int, _ int, _ int64, _ []int) bool {
		assert.True(s.T(), active)
		redirected = false
		return false
	})
	assert.False(s.T(), initDiverter())
	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestStopDiverter() {
	redirected = true
	monkey.Patch(gateway.SetConnectionDiverter, func(active bool, _ string, _ string, _ int, _ int, _ int, _ int64, _ []int) bool {
		assert.False(s.T(), active)
		redirected = false
		return false
	})
	stopDiverter()
	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestInitialCheckConnection_Proxy() {
	if runtime.GOOS == "linux" {
		s.T().Skipf("Proxy set not supported on linux")
		return
	}

	configuration.QPepConfig.General.PreferProxy = true
	validateConfiguration()

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		gateway.UsingProxy = true
		gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	redirected = false
	assert.False(s.T(), gateway.UsingProxy)
	initialCheckConnection()
	assert.True(s.T(), gateway.UsingProxy)
	assert.NotNil(s.T(), gateway.ProxyAddress)

	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestInitialCheckConnection_Diverter() {
	configuration.QPepConfig.General.PreferProxy = false
	validateConfiguration()
	if runtime.GOOS == "linux" {
		assert.False(s.T(), configuration.QPepConfig.General.PreferProxy)
	}

	redirected = false
	monkey.Patch(initDiverter, func() bool {
		redirected = true
		return true
	})

	assert.False(s.T(), gateway.UsingProxy)
	initialCheckConnection()
	assert.False(s.T(), gateway.UsingProxy)
	assert.Nil(s.T(), gateway.ProxyAddress)

	assert.True(s.T(), redirected)
}

func (s *ClientSuite) TestGatewayStatusCheck() {
	monkey.Patch(api.RequestEcho, func(string, string, int, bool) *api.EchoResponse {
		return &api.EchoResponse{
			Address:       "127.0.0.1",
			Port:          int64(s.testLocalListenPort),
			ServerVersion: "0.1.0",
		}
	})

	ok, resp := gatewayStatusCheck("127.0.0.1", "127.0.0.1", 8080)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "127.0.0.1", resp.Address)
	assert.Equal(s.T(), int64(s.testLocalListenPort), resp.Port)
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

func (s *ClientProxyListenerSuite) TestProxyListener_FailAccept() {
	listener, err := NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: s.testLocalListenPort,
	})

	s.T().Logf("err: %v", err)
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), listener)

	clListener := listener.(*ClientProxyListener)
	monkey.PatchInstanceMethod(reflect.TypeOf(clListener.base), "AcceptTCP",
		func(_ *net.TCPListener) (*net.TCPConn, error) {
			return nil, errors.ErrFailed
		})

	conn, errConn := clListener.Accept()
	assert.Nil(s.T(), conn)
	assert.Equal(s.T(), errors.ErrFailed, errConn)
}
