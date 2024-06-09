package notify

import (
	"fmt"
	"github.com/Project-Faster/qpep/qpep-tray/toast"
	"github.com/project-faster/dialog"
	"log"
)

var (
	MainIconData = ""
)

func NotifyUser(message, category string, longNotification bool) {
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
	return dialog.Message(str).YesNo()
}
