//go:build darwin

package notify

import (
	"fmt"
	"github.com/Project-Faster/qpep/shared/logger"
	gosx "github.com/Project-Faster/qpep/qpep-tray/notify/gosx-notifier"
	platformnotify "github.com/martinlindhe/notify"
	"github.com/project-faster/dialog"
)

var (
	MainIconData = ""
)

func NotifyUser(message, category string, longNotification bool) {
	n := &gosx.Notification{
		Sender:   "QPep",
		Title:    message,
		Subtitle: category,
	}
	n.Push()
}

func ErrorMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	logger.Error(str)

	platformnotify.Notify("QPep", "Error", str, MainIconData)
}
func InfoMsg(message string, parameters ...interface{}) {
	str := fmt.Sprintf(message, parameters...)
	logger.Info(str)
}
func ConfirmMsg(message string, parameters ...interface{}) bool {
	str := fmt.Sprintf(message, parameters...)
	logger.Info(str)
	return dialog.Message(str).YesNo()
}
