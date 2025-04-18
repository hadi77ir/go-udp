package log

import (
	"fmt"
	"os"

	"github.com/hadi77ir/go-logging"
)

var defaultLogger logging.Logger

func SetDefaultLogger(logger logging.Logger) {
	defaultLogger = logger
}
func DefaultLogger() logging.Logger {
	return defaultLogger
}
func Log(level logging.Level, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Log(level, args...)
	}
	if level == logging.PanicLevel {
		if defaultLogger == nil {
			fmt.Println(args)
		}
		os.Exit(1)
		panic("panic")
	}
}
