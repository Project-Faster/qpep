package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/getlantern/systray"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/qpep-tray/icons"
	"github.com/parvit/qpep/qpep-tray/toast"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/version"

	. "github.com/sqweek/dialog"
)

const (
	TooltipMsgDisconnected = "QPep TCP accelerator - Status: Disconnected"
	TooltipMsgConnecting   = "QPep TCP accelerator - Status: Connecting"
	TooltipMsgConnected    = "QPep TCP accelerator - Status: Connected"
)

var (
	ExeDir       = ""
	MainIconData = ""
)

func NotifyUser(message, category string, longNotification bool) {
	if len(MainIconData) == 0 {
		MainIconData = filepath.Join(ExeDir, "main.png")
		// extract the icon for notifications
		if err := ioutil.WriteFile(MainIconData, icons.MainIconConnected, 0666); err != nil {
			NotifyUser("Could not extract notification icon", "Error", false)
		}
	}
	var duration = toast.Short
	if longNotification {
		duration = toast.Long
	}
	n := toast.Notification{
		AppID:    "QPep",
		Title:    category,
		Message:  message,
		Duration: duration,
		Icon:     MainIconData,

		ActivationType: "background",
	}
	if err := n.Push(); err != nil {
		log.Println("ERR: ", err)
	}
}

func ErrorMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	log.Println("ERR: ", str)

	NotifyUser(message, "Error", false)
}
func InfoMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	log.Println("INFO: ", str)
}
func ConfirmMsg(message string, parameters ...interface{}) bool {
	str := fmt.Sprintf(message, parameters...)
	log.Println("ASK: ", str)
	return Message(str).YesNo()
}

var contextConfigWatchdog context.Context
var cancelConfigWatchdog context.CancelFunc

var contextConnectionWatchdog context.Context
var cancelConnectionWatchdog context.CancelFunc

var addressCheckBoxList []*systray.MenuItem

var mClient *systray.MenuItem
var mServer *systray.MenuItem

func onReady() {
	// Setup tray menu
	systray.SetTemplateIcon(icons.MainIconData, icons.MainIconData)
	systray.SetTitle("QPep Connection Accelerator")
	systray.SetTooltip("QPep Connection Accelerator")

	mInfo := systray.AddMenuItem("About", "About the project")
	systray.AddSeparator()
	mStatus := systray.AddMenuItem("Status Interface", "Open the status web gui")
	mConfig := systray.AddMenuItem("Edit Configuration", "Open configuration for next client / server executions")
	mConfigRefresh := systray.AddMenuItem("Reload Configuration", "Reload configuration from disk and restart the service")
	systray.AddSeparator()
	mListeningAddress := systray.AddMenuItem("Listen Address", "Force a listening address on the fly")
	addressList, _ := shared.GetLanListeningAddresses()
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

	// launch the watchdog routines
	contextConfigWatchdog, cancelConfigWatchdog = startReloadConfigurationWatchdog()
	contextConnectionWatchdog, cancelConnectionWatchdog = startConnectionStatusWatchdog()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v", err)
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
						InfoMsg(fmt.Sprintf("Listening address will be forced to %s", addressList[index]))
					}
				}
			}(box, idx)
		}

		NotifyUser("Ready", "Info", false)

		for {
			select {
			case <-mConfig.ClickedCh:
				openConfigurationWithOSEditor()
				continue

			case <-mStatus.ClickedCh:
				openWebguiWithOSBrowser(mClientActive, mServerActive)
				continue

			case <-mInfo.ClickedCh:
				NotifyUser(version.Version(), "Version", true)
				continue

			case <-mConfigRefresh.ClickedCh:
				shared.ReadConfiguration(true)
				NotifyUser("Reload finished", "Info", false)
				continue

			case <-mClient.ClickedCh:
				if !mClientActive {
					if startClient() == nil {
						NotifyUser("Start Client", "Info", false)
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
						NotifyUser("Stop Client", "Info", false)
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
					NotifyUser("Start Server", "Info", false)
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
					NotifyUser("Stop Server", "Info", false)
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
					if ok := ConfirmMsg("Do you want to quit QPep and stop its services?"); !ok {
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

func onExit() {
	log.Println("Waiting for resources to be freed...")

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

	log.Println("Closed")
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
				log.Printf("PANIC: %v\n", err)
				debug.PrintStack()
			}
		}()

		var flip = 0
		var animIcons = [][]byte{
			icons.MainIconWaiting,
			icons.MainIconData,
		}

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
						NotifyUser("Disconnected", "Info", false)
					}
					state = stateDisconnected
					continue
				}
				if state == stateDisconnected {
					state = stateConnecting
					systray.SetTooltip(TooltipMsgConnecting)
					flip = 0
					NotifyUser("Initiating connection...", "Info", false)
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
				log.Printf("PANIC: %v\n", err)
				debug.PrintStack()
			}
		}()

	CHECKLOOP:
		for {
			select {
			case <-ctx.Done():
				log.Println("Stopping connection check watchdog")
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

				listenHost := shared.QPepConfig.ListenHost
				gatewayHost := shared.QPepConfig.GatewayHost
				gatewayAPIPort := shared.QPepConfig.GatewayAPIPort

				shared.UsingProxy, shared.ProxyAddress = shared.GetSystemProxyEnabled()
				if shared.UsingProxy {
					shared.QPepConfig.ListenHost = shared.ProxyAddress.Hostname()
				}
				log.Printf("Proxy: %v %v\n", shared.UsingProxy, shared.ProxyAddress)

				if !fakeAPICallCheckProxy() {
					NotifyUser("Detected issue with setting the proxy values, terminating...", "Error", false)
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
						log.Printf("Server Echo: FAILED\n")
						continue
					}

					log.Printf("Server Echo: %s %d\n", resp.Address, resp.Port)

					NotifyUser("Connection established", "Info", false)
					state = stateConnected
				}
				continue
			}
		}
	}()

	return ctx, cancel
}

// fakeAPICallCheckProxy executes a "fake" api call to the local server to check for the connection running through
// the global proxy, this is checked by the client that adds the "X-QPEP-PROXY" header with value "true", a missing or
// "false" value means the proxy is not running correctly
func fakeAPICallCheckProxy() bool {
	data, err, _ := shared.RunCommand("powershell.exe", "-ExecutionPolicy", "ByPass", "-Command",
		fmt.Sprintf("Invoke-WebRequest -Uri \"http://%s:%d/qpep-client-proxy-check\" -UseBasicParsing -TimeoutSec 1", shared.QPepConfig.ListenHost, shared.QPepConfig.ListenPort),
	)
	logger.Info("proxy check data: %s", data)
	logger.Info("proxy check error: %v", err)
	if err != nil {
		return false
	}
	if strings.Contains(string(data), "X-QPEP-PROXY, true") {
		logger.Info("proxy is working")
		return true
	}
	return false
}
