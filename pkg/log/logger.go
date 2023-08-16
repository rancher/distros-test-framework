package log

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	once sync.Once
	le   *log.Entry
)

func AddLogger(json bool) *log.Entry {
	once.Do(func() {
		logger := newLogger(json)
		le = log.NewEntry(logger)
	})

	return le
}

func newLogger(json bool) *log.Logger {
	logger := log.New()

	if !json {
		logger.SetFormatter(&log.TextFormatter{
			ForceColors:   true,
			FullTimestamp: true,
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				return f.Function, fmt.Sprintf("%s:%d", f.File, f.Line)
			},
			QuoteEmptyFields: true,
		})
	} else {
		logger.SetFormatter(&log.JSONFormatter{
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				return f.Function, fmt.Sprintf("%s:%d", f.File, f.Line)
			},
			PrettyPrint: true,
		})
	}

	logger.SetReportCaller(true)
	logger.SetLevel(log.DebugLevel)
	logger.Out = os.Stdout

	return logger
}
