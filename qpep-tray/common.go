package main

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/getlantern/systray"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/qpep-tray/icons"
	"github.com/parvit/qpep/shared"

	. "github.com/sqweek/dialog"
)

const (
	TooltipMsgDisconnected = "QPep TCP accelerator - Status: Disconnected"
	TooltipMsgConnecting   = "QPep TCP accelerator - Status: Connecting"
	TooltipMsgConnected    = "QPep TCP accelerator - Status: Connected"
)

func ErrorMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	log.Println("ERR: ", str)
	Message(str).Error()
}
func InfoMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	log.Println("INFO: ", str)
	Message(str).Info()
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

func onReady() {
	// Setup tray menu
	systray.SetTemplateIcon(icons.MainIconData, icons.MainIconData)
	systray.SetTitle("QPep TCP accelerator")
	systray.SetTooltip("QPep TCP accelerator")

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
	mClient := systray.AddMenuItemCheckbox("Activate Client", "Launch/Stop QPep Client", false)
	mServer := systray.AddMenuItemCheckbox("Activate Server", "Launch/Stop QPep Server", false)
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop all and quit the whole app")

	// Sets the icon of the menu items
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
						InfoMsg("Listening address will be forced to %s", addressList[index])
					}
				}
			}(box, idx)
		}

		for {
			select {
			case <-mConfig.ClickedCh:
				openConfigurationWithOSEditor()
				continue

			case <-mStatus.ClickedCh:
				openWebguiWithOSBrowser(mClientActive, mServerActive)
				continue

			case <-mConfigRefresh.ClickedCh:
				shared.ReadConfiguration(true)
				continue

			case <-mClient.ClickedCh:
				if !mClientActive {
					if ok := ConfirmMsg("Do you want to enable the client?"); !ok {
						break
					}
					if startClient() == nil {
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
					if ok := ConfirmMsg("Do you want to disable the client?"); !ok {
						break
					}
					if stopClient() == nil {
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
					if ok := ConfirmMsg("Do you want to enable the server?"); !ok {
						break
					}
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
					if ok := ConfirmMsg("Do you want to enable the server?"); !ok {
						break
					}
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

	log.Println("Closing...")
}

func startConnectionStatusWatchdog() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	const (
		stateDisconnected = 0
		stateConnecting   = 1
		stateConnected    = 2
	)

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v\n", err)
				debug.PrintStack()
			}
		}()

		var state = stateDisconnected
		var pubAddress = ""
		var flip = 0
		var animIcons = [][]byte{
			icons.MainIconWaiting,
			icons.MainIconData,
		}

	CHECKLOOP:
		for {
			select {
			case <-ctx.Done():
				log.Println("Stopping connection check watchdog")
				break CHECKLOOP

			case <-time.After(1 * time.Second):
				if !clientActive && !serverActive {
					state = stateDisconnected
					pubAddress = ""
					systray.SetTemplateIcon(icons.MainIconData, icons.MainIconData)
					systray.SetTooltip(TooltipMsgDisconnected)
					continue
				}
				if state == stateDisconnected {
					state = stateConnecting
					systray.SetTooltip(TooltipMsgConnecting)
					flip = 0
				}

				// Inverse of what one might expect
				// Client -> Server: url must contain "/server", so flag true
				// Server -> Server: url must contain "/server", so flag true
				// All else false so url contains "/client"
				var clientToServer = (!serverActive && clientActive) || (serverActive && !clientActive)

				listenHost := shared.QPepConfig.ListenHost
				gatewayHost := shared.QPepConfig.GatewayHost
				gatewayAPIPort := shared.QPepConfig.GatewayAPIPort

				if state != stateConnected {
					var resp = api.RequestEcho(listenHost, gatewayHost, gatewayAPIPort, clientToServer)
					if resp == nil {
						systray.SetTemplateIcon(animIcons[flip], animIcons[flip])
						flip = (flip + 1) % 2

						// check in tray-icon for activated proxy
						shared.UsingProxy, shared.ProxyAddress = shared.GetSystemProxyEnabled()
						if shared.UsingProxy {
							shared.QPepConfig.ListenHost = shared.ProxyAddress.Hostname()
						}
						log.Printf("Proxy: %v %v\n", shared.UsingProxy, shared.ProxyAddress)
						log.Printf("Server Echo: FAILED\n")
						continue
					}

					log.Printf("Server Echo: %s %d\n", resp.Address, resp.Port)
					pubAddress = resp.Address
				}

				if len(pubAddress) > 0 {
					var status = api.RequestStatus(listenHost, gatewayHost, gatewayAPIPort, pubAddress, clientToServer)
					if status == nil {
						log.Printf("Server Status: no / invalid response\n")
					} else if status.ConnectionCounter < 0 {
						log.Printf("Server Status: no connections received\n")
					}
					if status == nil || status.ConnectionCounter < 0 {
						pubAddress = ""
						state = stateConnecting
						systray.SetTemplateIcon(animIcons[flip], animIcons[flip])
						flip = (flip + 1) % 2
						continue
					}

					log.Printf("Server Status: %s %d\n", status.LastCheck, status.ConnectionCounter)
					state = stateConnected
					systray.SetTemplateIcon(icons.MainIconConnected, icons.MainIconConnected)
					systray.SetTooltip(TooltipMsgConnected)
				}
				continue
			}
		}
	}()

	return ctx, cancel
}
