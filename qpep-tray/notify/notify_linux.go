//go:build linux

package notify

import (
	"fmt"
	platformnotify "github.com/martinlindhe/notify"
	"github.com/project-faster/dialog"
	"log"
)

var (
	MainIconData = ""
)

func NotifyUser(message, category string, longNotification bool) {
	platformnotify.Notify("QPep", category, message, MainIconData)
}

func ErrorMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	log.Println("ERR: ", str)

	platformnotify.Notify("QPep", "Error", str, MainIconData)
}
func InfoMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	log.Println("INFO: ", str)
}
func ConfirmMsg(message string, parameters ...interface{}) bool {
	str := fmt.Sprintf(message, parameters...)
	log.Println("ASK: ", str)
	return dialog.Message(str).YesNo()
}
