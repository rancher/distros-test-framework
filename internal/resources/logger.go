package resources

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/logger"
)

var log = logger.AddLogger()

// LogLevel logs the message with the specified level.
func LogLevel(level, format string, args ...interface{}) {
	msg := formatLogArgs(format, args...)

	envLogLevel := os.Getenv("LOG_LEVEL")
	envLogLevel = strings.ToLower(envLogLevel)

	switch level {
	case "debug":
		if envLogLevel == "debug" {
			log.Debug(msg)
		} else {
			return
		}
	case "info":
		if envLogLevel == "info" || envLogLevel == "" || envLogLevel == "debug" {
			log.Info(msg)
		} else {
			return
		}
	case "warn":
		if envLogLevel == "warn" || envLogLevel == "" || envLogLevel == "info" || envLogLevel == "debug" {
			log.Warn(msg)
		} else {
			return
		}
	case "error":
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			funcName := runtime.FuncForPC(pc).Name()
			log.Error(fmt.Sprintf("%s\nLast call: %s in %s:%d", msg, funcName, file, line))
		}
		log.Error(msg)
	case "fatal":
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			funcName := runtime.FuncForPC(pc).Name()
			log.Fatal(fmt.Sprintf("%s\nLast call: %s in %s:%d", msg, funcName, file, line))
		}
		log.Fatal(msg)
	default:
		log.Info(msg)
	}
}

// ReturnLogError logs the error and returns it.
func ReturnLogError(format string, args ...interface{}) error {
	err := formatLogArgs(format, args...)
	if err != nil {
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			funcName := runtime.FuncForPC(pc).Name()

			formattedPath := fmt.Sprintf("file:%s:%d", file, line)
			log.Error(fmt.Sprintf("%s\nLast call: %s in %s", err.Error(), funcName, formattedPath))
		} else {
			log.Error(err.Error())
		}
	}

	return err
}

// formatLogArgs formats the logger message.
func formatLogArgs(format string, args ...interface{}) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", format)
	}
	if e, ok := args[0].(error); ok {
		if len(args) > 1 {
			return fmt.Errorf(format, args[1:]...)
		}

		return e
	}

	return fmt.Errorf(format, args...)
}
