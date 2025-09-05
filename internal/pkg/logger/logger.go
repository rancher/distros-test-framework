package logger

import (
	"fmt"
	"runtime"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	once sync.Once
	le   *log.Entry
)

func AddLogger() *log.Entry {
	once.Do(func() {
		logger := newLogger()
		le = log.NewEntry(logger)
	})

	return le
}

func newLogger() *log.Logger {
	logger := log.New()
	logger.SetFormatter(customFormatter())
	logger.SetReportCaller(true)
	logger.SetLevel(log.DebugLevel)

	return logger
}

func customFormatter() *log.TextFormatter {
	if log.GetLevel() == log.DebugLevel || log.GetLevel() == log.InfoLevel {
		return &log.TextFormatter{
			ForceColors:   true,
			FullTimestamp: true,
			CallerPrettyfier: func(_ *runtime.Frame) (string, string) {
				return "", ""
			},
			QuoteEmptyFields: true,
		}
	} else if log.GetLevel() == log.WarnLevel || log.GetLevel() == log.ErrorLevel ||
		log.GetLevel() == log.FatalLevel || log.GetLevel() == log.PanicLevel {
		return &log.TextFormatter{
			ForceColors:   true,
			FullTimestamp: true,
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				return fmt.Sprintf("%s:%d", f.File, f.Line), ""
			},
			QuoteEmptyFields: true,
		}
	}

	return nil
}
