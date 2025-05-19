package inmemory

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-logr/logr"
)

var _ badger.Logger = &LoggerShim{}

type LoggerShim struct {
	log logr.Logger
}

func NewLoggerShim(log logr.Logger) *LoggerShim {
	return &LoggerShim{log: log}
}

func (l *LoggerShim) Errorf(format string, args ...interface{}) {
	l.log.Error(fmt.Errorf(format, args...), "badger error")
}

func (l *LoggerShim) Warningf(format string, args ...interface{}) {
	l.log.Info(fmt.Sprintf(format, args...))
}

func (l *LoggerShim) Infof(format string, args ...interface{}) {
	l.log.V(1).Info(fmt.Sprintf(format, args...))
}

func (l *LoggerShim) Debugf(format string, args ...interface{}) {
	l.log.V(1).Info(fmt.Sprintf(format, args...))
}
