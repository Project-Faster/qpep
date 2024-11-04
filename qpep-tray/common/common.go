package common

import (
	"context"
	"fmt"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/shared/version"
	"github.com/Project-Faster/qpep/workers/gateway"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/qpep-tray/icons"
	"github.com/Project-Faster/qpep/qpep-tray/notify"
	"github.com/project-faster/systray"
)

const (
	TooltipMsgDisconnected = "QPep TCP accelerator - Status: Disconnected"
	TooltipMsgConnecting   = "QPep TCP accelerator - Status: Connecting"
	TooltipMsgConnected    = "QPep TCP accelerator - Status: Connected"
)

var (
	ExeDir = ""
)

func Init(pathDir string) {
	logger.SetupLogger("qpep-tray.log", "info")

	ExeDir, _ = filepath.Abs(filepath.Dir(pathDir))

	if runtime.GOOS != "darwin" {
		notify.MainIconData = filepath.Join(ExeDir, "main.png")
		// extract the icon for notifications
		if err := ioutil.WriteFile(notify.MainIconData, icons.MainIconConnected, 0666); err != nil {
			logger.Error("Could not extract notification icon")
		}
	}
}

var contextConfigWatchdog context.Context
var cancelConfigWatchdog context.CancelFunc

var contextConnectionWatchdog context.Context
var cancelConnectionWatchdog context.CancelFunc

var addressCheckBoxList []*systray.MenuItem

var mClient *systray.MenuItem
var mServer *systray.MenuItem

func OnReady() {
	// Setup tray menu
	systray.SetTemplateIcon(icons.MainIconData, icons.MainIconData)
	if runtime.GOOS == "windows" {
		systray.SetTitle("QPep Connection Accelerator")
	}
	systray.SetTooltip("QPep Connection Accelerator")

	mInfo := systray.AddMenuItem("About", "About the project")
	systray.AddSeparator()
	mStatus := systray.AddMenuItem("Status Interface", "Open the status web gui")
	mConfig := systray.AddMenuItem("Edit Configuration", "Open configuration for next client / server executions")
	mConfigRefresh := systray.AddMenuItem("Reload Configuration", "Reload configuration from disk and restart the service")
	mLogs := systray.AddMenuItem("Open Logs Folder", "Opens the logs folder for checking")
	systray.AddSeparator()
	mListeningAddress := systray.AddMenuItem("Listen Address", "Force a listening address on the fly")
	addressList, _ := gateway.GetLanListeningAddresses()
	for _, addr := range addressList {
		box := mListeningAddress.AddSubMenuItemCheckbox(addr, "Force listening address to be "+addr, false)
		addressCheckBoxList = append(addressCheckBoxList, box)
	}
	systray.AddSeparator()
	mClient = systray.AddMenuItemCheckbox("Activate Client", "Launch/Stop QPep Client", false)
	mServer = systray.AddMenuItemCheckbox("Activate Server", "Launch/Stop QPep Server", false)
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop all and quit the whole app")

	// Sets the icon of the menu items
	mInfo.SetIcon(icons.MainIconConnected)
	mQuit.SetIcon(icons.ExitIconData)
	mStatus.SetIcon(icons.ConfigIconData)
	mConfig.SetIcon(icons.ConfigIconData)
	mConfigRefresh.SetIcon(icons.RefreshIconData)
	mLogs.SetIcon(icons.ConfigIconData)

	// launch the watchdog routines
	contextConfigWatchdog, cancelConfigWatchdog = startReloadConfigurationWatchdog()
	contextConnectionWatchdog, cancelConnectionWatchdog = startConnectionStatusWatchdog()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Panic("PANIC: %v", err)
				debug.PrintStack()
				cancelConfigWatchdog()
			}
		}()

		mClientActive := false
		mServerActive := false

		// check clicks on address checkboxes
		for idx, box := range addressCheckBoxList {
			go func(self *systray.MenuItem, index int) {
				for {
					select {
					case <-self.ClickedCh:
						for _, checkbox := range addressCheckBoxList {
							if checkbox == self {
								checkbox.Check()
								continue
							}
							checkbox.Uncheck()
						}
						notify.InfoMsg(fmt.Sprintf("Listening address will be forced to %s", addressList[index]))
					}
				}
			}(box, idx)
		}

		notify.NotifyUser("Ready", "Info", false)

		for {
			select {
			case <-mConfig.ClickedCh:
				openConfigurationWithOSEditor()
				continue

			case <-mStatus.ClickedCh:
				openWebguiWithOSBrowser(mClientActive, mServerActive)
				continue

			case <-mInfo.ClickedCh:
				notify.NotifyUser(version.Version(), "Version", true)
				continue

			case <-mConfigRefresh.ClickedCh:
				configuration.ReadConfiguration(true)
				notify.NotifyUser("Reload finished", "Info", false)
				continue

			case <-mLogs.ClickedCh:
				openLogsFolder()
				continue

			case <-mClient.ClickedCh:
				if !mClientActive {
					if startClient() == nil {
						notify.NotifyUser("Start Client", "Info", false)
						mClientActive = true
						mClient.SetTitle("Stop Client")
						mClient.Enable()
						mClient.Check()

						mServerActive = false
						mServer.SetTitle("Activate Server")
						mServer.Uncheck()
						mServer.Disable()
						stopServer()
					}

				} else {
					if stopClient() == nil {
						notify.NotifyUser("Stop Client", "Info", false)
						mClientActive = false
						mClient.SetTitle("Activate Client")
						mClient.Enable()
						mClient.Uncheck()

						mServerActive = false
						mServer.SetTitle("Activate Server")
						mServer.Uncheck()
						mServer.Enable()
						stopServer()
					}
				}

			case <-mServer.ClickedCh:
				if !mServerActive {
					notify.NotifyUser("Start Server", "Info", false)
					mServerActive = true
					mServer.SetTitle("Stop Server")
					mServer.Enable()
					mServer.Check()
					startServer()

					mClientActive = false
					mClient.SetTitle("Activate Client")
					mClient.Uncheck()
					mClient.Disable()
					stopClient()
				} else {
					notify.NotifyUser("Stop Server", "Info", false)
					mServerActive = false
					mServer.SetTitle("Activate Server")
					mServer.Enable()
					mServer.Uncheck()
					stopServer()

					mClientActive = false
					mClient.SetTitle("Activate Client")
					mClient.Uncheck()
					mClient.Enable()
					stopClient()
				}

			case <-mQuit.ClickedCh:
				if mServerActive || mClientActive {
					if ok := notify.ConfirmMsg("Do you want to quit QPep and stop its services?"); !ok {
						break
					}
					stopClient()
					stopServer()
				}
				systray.Quit()
				return
			}
		}
	}()
}

func OnExit() {
	logger.Info("Waiting for resources to be freed...")

	// request cancelling of the watchdogs
	cancelConfigWatchdog()
	cancelConnectionWatchdog()

	select {
	case <-time.After(10 * time.Second):
		break
	case <-contextConfigWatchdog.Done():
		break
	}

	select {
	case <-time.After(10 * time.Second):
		break
	case <-contextConnectionWatchdog.Done():
		break
	}

	notify.NotifyUser("Closed", "Info", false)
}

func startConnectionStatusWatchdog() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	const (
		stateDisconnected = 0
		stateConnecting   = 1
		stateConnected    = 2
	)

	var state = stateDisconnected

	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Panic("PANIC: %v\n", err)
				debug.PrintStack()
			}
		}()

		var flip = 0
		var animIcons = getWaitingIcons()

	ICONLOOP:
		for {
			select {
			case <-ctx.Done():
				break ICONLOOP

			case <-time.After(1 * time.Second):
				if !clientActive && !serverActive {
					systray.SetTemplateIcon(icons.MainIconData, icons.MainIconData)
					systray.SetTooltip(TooltipMsgDisconnected)
					if state != stateDisconnected {
						notify.NotifyUser("Disconnected", "Info", false)
					}
					state = stateDisconnected
					continue
				}
				if state == stateDisconnected {
					state = stateConnecting
					systray.SetTooltip(TooltipMsgConnecting)
					flip = 0
					notify.NotifyUser("Initiating connection...", "Info", false)
				}
				if state == stateConnected {
					systray.SetTemplateIcon(icons.MainIconConnected, icons.MainIconConnected)
					systray.SetTooltip(TooltipMsgConnected)
					continue
				}
				systray.SetTemplateIcon(animIcons[flip], animIcons[flip])
				flip = (flip + 1) % 2
				break
			}
		}
	}()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Panic("PANIC: %v\n", err)
				debug.PrintStack()
			}
		}()

	CHECKLOOP:
		for {
			select {
			case <-ctx.Done():
				logger.Info("Stopping connection check watchdog")
				break CHECKLOOP

			case <-time.After(3 * time.Second):
				if !clientActive && !serverActive {
					state = stateDisconnected
					continue
				}

				// Inverse of what one might expect
				// Client -> Server: url must contain "/server", so flag true
				// Server -> Server: url must contain "/server", so flag true
				// All else false so url contains "/client"
				var clientToServer = (!serverActive && clientActive) || (serverActive && !clientActive)

				listenHost := configuration.QPepConfig.Client.LocalListeningAddress
				gatewayHost := configuration.QPepConfig.Client.GatewayHost
				gatewayAPIPort := configuration.QPepConfig.General.APIPort

				gateway.UsingProxy, gateway.ProxyAddress = gateway.GetSystemProxyEnabled()
				if gateway.UsingProxy {
					listenHost = gateway.ProxyAddress.Hostname()
				}
				logger.Info("Proxy: %v %v\n", gateway.UsingProxy, gateway.ProxyAddress)

				if !fakeAPICallCheckProxy() {
					notify.NotifyUser("Detected issue with setting the proxy values, terminating...", "Error", false)
					state = stateDisconnected
					if clientActive {
						mClient.ClickedCh <- struct{}{}
						continue
					}
				}

				if state != stateConnected {
					var resp = api.RequestEcho(listenHost, gatewayHost, gatewayAPIPort, clientToServer)
					if resp == nil {
						// check in tray-icon for activated proxy
						logger.Info("Server Echo: FAILED\n")
						continue
					}

					logger.Info("Server Echo: OK\n")
					notify.NotifyUser("Connection established", "Info", false)
					state = stateConnected
				}
				continue
			}
		}
	}()

	return ctx, cancel
}
