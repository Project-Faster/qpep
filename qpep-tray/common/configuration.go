package common

import (
	"context"
	"fmt"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/logger"
	"github.com/parvit/qpep/qpep-tray/notify"
	"os"
	"time"

	"github.com/skratchdot/open-golang/open"
)

func openConfigurationWithOSEditor() {
	_, baseConfigPath, _ := configuration.GetConfigurationPaths()

	if err := open.Run(baseConfigPath); err != nil {
		notify.ErrorMsg("Editor configuration failed with error: %v", err)
		return
	}
}

func openWebguiWithOSBrowser(clientMode, serverMode bool) {
	mode := "server"
	port := configuration.QPepConfig.General.APIPort
	if (clientMode && serverMode) || (!clientMode && !serverMode) {
		notify.ErrorMsg("Webgui can start with just one mode between server and client!")
		return
	}
	if clientMode {
		mode = "client"
	}
	if serverMode {
		mode = "server"
	}

	guiurl := fmt.Sprintf(configuration.WEBGUI_URL, port, mode, port)
	if err := open.Run(guiurl); err != nil {
		notify.ErrorMsg("Webgui startup failed with error: %v", err)
		return
	}
}

func startReloadConfigurationWatchdog() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_, baseConfigFile, _ := configuration.GetConfigurationPaths()

		var lastModTime time.Time
		if stat, err := os.Stat(baseConfigFile); err == nil {
			lastModTime = stat.ModTime()

		} else {
			notify.ErrorMsg("Configuration file not found, stopping")
			cancel()
			return
		}

	CHECKLOOP:
		for {
			select {
			case <-ctx.Done():
				logger.Info("Stopping config file watchdog")
				break CHECKLOOP

			case <-time.After(10 * time.Second):
				stat, err := os.Stat(baseConfigFile)
				if err != nil {
					continue
				}
				if !stat.ModTime().After(lastModTime) {
					continue
				}
				lastModTime = stat.ModTime()
				if configuration.ReadConfiguration(true) == nil {
					reloadClientIfRunning()
					reloadServerIfRunning()
				}
				continue
			}
		}
	}()

	return ctx, cancel
}
