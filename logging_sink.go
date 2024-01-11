package stats

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

type loggingSink struct {
	writer io.Writer
	now    func() time.Time
}

type logLine struct {
	Level     string                `json:"level"`
	Timestamp sixDecimalPlacesFloat `json:"ts"`
	Logger    string                `json:"logger"`
	Message   string                `json:"msg"`
	JSON      map[string]string     `json:"json"`
}

type sixDecimalPlacesFloat float64

func (f sixDecimalPlacesFloat) MarshalJSON() ([]byte, error) {
	var ret []byte
	ret = strconv.AppendFloat(ret, float64(f), 'f', 6, 64)
	return ret, nil
}

// NewLoggingSink returns a "default" logging Sink that flushes stats
// to os.StdErr. This sink is not fast, or flexible, it doesn't
// buffer, it exists merely to be convenient to use by default, with
// no configuration.
//
// The format of this logger is similar to Zap, but not explicitly
// importing Zap to avoid the dependency. The format is as if you used
// a zap.NewProduction-generated logger, but also added a
// log.With(zap.Namespace("json")). This does not include any
// stacktrace for errors at the moment.
//
// If these defaults do not work for you, users should provide their
// own logger, conforming to FlushableSink, instead.
func NewLoggingSink() FlushableSink {
	return &loggingSink{writer: os.Stderr, now: time.Now}
}

// this is allocated outside of logMessage, even though its only used
// there, to avoid allocing a map every time we log.
var emptyMap = map[string]string{}

func (s *loggingSink) logMessage(level string, msg string) {
	nanos := s.now().UnixNano()
	sec := sixDecimalPlacesFloat(float64(nanos) / float64(time.Second))
	enc := json.NewEncoder(s.writer)
	enc.Encode(logLine{
		Message:   msg,
		Level:     level,
		Timestamp: sec,
		Logger:    "gostats.loggingsink",
		// intentional empty map used to avoid any null parsing issues
		// on the log collection side
		JSON: emptyMap,
	})
}

func (s *loggingSink) log(name, typ string, value float64) {
	nanos := s.now().UnixNano()
	sec := sixDecimalPlacesFloat(float64(nanos) / float64(time.Second))
	enc := json.NewEncoder(s.writer)
	kv := map[string]string{
		"type":  typ,
		"value": fmt.Sprintf("%f", value),
	}
	if name != "" {
		kv["name"] = name
	}
	enc.Encode(logLine{
		Message:   fmt.Sprintf("flushing %s", typ),
		Level:     "debug",
		Timestamp: sec,
		Logger:    "gostats.loggingsink",
		JSON:      kv,
	})
}

func (s *loggingSink) FlushCounter(name string, value uint64) { s.log(name, "counter", float64(value)) }

func (s *loggingSink) FlushGauge(name string, value uint64) { s.log(name, "gauge", float64(value)) }

func (s *loggingSink) FlushTimer(name string, value float64) { s.log(name, "timer", value) }

func (s *loggingSink) Flush() { s.log("", "all stats", 0) }

// Logger

func (s *loggingSink) Errorf(msg string, args ...interface{}) {
	s.logMessage("error", fmt.Sprintf(msg, args...))
}

func (s *loggingSink) Warnf(msg string, args ...interface{}) {
	s.logMessage("warn", fmt.Sprintf(msg, args...))
}
