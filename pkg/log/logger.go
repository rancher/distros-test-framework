package log

import (
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	once sync.Once
	le   *log.Entry
)

func AddLogger(json bool) *log.Entry {
	once.Do(func() {
		logger := addLogger(json)
		le = log.NewEntry(logger)
	})

	return le
}

func addLogger(json bool) *log.Logger {
	logger := log.New()

	if !json {
		logger.SetFormatter(&log.TextFormatter{
			ForceColors:   true,
			FullTimestamp: true,
		})
	} else {
		logger.SetFormatter(&log.JSONFormatter{})
	}

	logger.SetReportCaller(true)
	logger.SetLevel(log.DebugLevel)
	logger.Out = os.Stdout

	return logger
}
