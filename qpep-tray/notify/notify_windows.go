package notify

import (
	"fmt"
	"github.com/parvit/qpep/qpep-tray/notify/toast"
	"github.com/parvit/qpep/shared/logger"
	"github.com/project-faster/dialog"
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
		logger.Error("ERR: %v", err)
	}
}

func ErrorMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	logger.Error(str)

	NotifyUser(str, "Error", false)
}
func InfoMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	logger.Info("ASK: %s", str)
}
func ConfirmMsg(message string, parameters ...interface{}) bool {
	str := fmt.Sprintf(message, parameters...)
	logger.Info("ASK: %s", str)
	return dialog.Message(str).YesNo()
}
