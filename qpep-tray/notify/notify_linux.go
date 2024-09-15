//go:build linux

package notify

import (
	"fmt"
	platformnotify "github.com/martinlindhe/notify"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/project-faster/dialog"
)

var (
	MainIconData = ""
)

func NotifyUser(message, category string, longNotification bool) {
	platformnotify.Notify("QPep", category, message, MainIconData)
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
	logger.Info("ASK: %s", str)
	return dialog.Message(str).YesNo()
}
