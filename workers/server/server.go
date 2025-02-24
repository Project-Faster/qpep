package server

import (
	"context"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/workers/gateway"
	"runtime/debug"
	"sync"
	"time"
)

// RunServer method validates the provided server configuration and then launches the server
// with the input context
func RunServer(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
		if quicListener != nil {
			quicListener.Close(0, "")
		}
		cancel()
	}()

	// update configuration from flags
	validateConfiguration()

	if configuration.QPepConfig.Analytics.Enabled {
		api.Statistics.Start(configuration.QPepConfig.Analytics)
	}
	defer api.Statistics.Stop()

	config := configuration.QPepConfig.Server
	logger.Info("Opening QPEP Server on: %s:%d\n", config.LocalListeningAddress, config.LocalListenPort)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	// launches listener
	go func() {
		defer func() {
			wg.Done()
		}()
		//defer wg.Done()
		listenQuicSession(ctx, cancel, config.LocalListeningAddress, config.LocalListenPort)
	}()

	go func() {
		defer func() {
			wg.Done()
		}()
		//defer wg.Done()
		performanceWatcher(ctx, cancel)
	}()

	// termination wait
	wg.Wait()
}

// performanceWatcher method is a goroutine that checks the current speed of every host every second and
// updates the values for the current speed and total number of bytes uploaded / downloaded
func performanceWatcher(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			hosts := api.Statistics.GetHosts()

			for _, host := range hosts {
				// load the current count and reset it atomically (so there's no race condition)
				dwCount := api.Statistics.GetCounterAndClear(api.PERF_DW_COUNT, host)
				upCount := api.Statistics.GetCounterAndClear(api.PERF_UP_COUNT, host)

				// update the speeds and totals for the client
				if dwCount >= 0.0 {
					api.Statistics.SetCounter(dwCount/1024.0, api.PERF_DW_SPEED, host)
					api.Statistics.IncrementCounter(dwCount, api.PERF_DW_TOTAL, host)
				}

				if upCount >= 0.0 {
					api.Statistics.SetCounter(upCount/1024.0, api.PERF_UP_SPEED, host)
					api.Statistics.IncrementCounter(upCount, api.PERF_UP_TOTAL, host)
				}
			}
		}
	}
}

// validateConfiguration method checks the validity of the configuration values that have been provided, panicking if
// there is any issue
func validateConfiguration() {
	configSrv := configuration.QPepConfig.Server
	configGeneral := configuration.QPepConfig.General
	configProto := configuration.QPepConfig.Protocol
	configBroker := configuration.QPepConfig.Analytics

	configuration.AssertParamIP("listen host", configSrv.LocalListeningAddress)
	configuration.AssertParamPort("listen port", configSrv.LocalListenPort)

	configSrv.LocalListeningAddress, _ = gateway.GetDefaultLanListeningAddress(configSrv.LocalListeningAddress, "")

	if configSrv.ExternalListeningAddress != "" {
		configuration.AssertParamIP("external source host", configSrv.ExternalListeningAddress)
	} else {
		configSrv.ExternalListeningAddress = configSrv.LocalListeningAddress
	}

	configuration.AssertParamPort("api port", configGeneral.APIPort)
	configuration.AssertParamValidTimeout("idle timeout", configProto.IdleTimeout)
	configuration.AssertParamPortsDifferent("ports", configSrv.LocalListenPort, configGeneral.APIPort)

	if !configBroker.Enabled {
		configBroker.Enabled = false

	} else {
		configuration.AssertParamIP("broker address", configBroker.BrokerAddress)
		configuration.AssertParamPort("broker port", configBroker.BrokerPort)
		configuration.AssertParamString("broker topic", configBroker.BrokerTopic)
		configuration.AssertParamChoice("broker protocol", configBroker.BrokerProtocol, []string{"tcp", "udp"})
	}

	logger.Info("Server configuration validation OK\n")
}
