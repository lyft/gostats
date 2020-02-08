package stats

import (
	"fmt"
	"os"
	"strconv"
)

const (
	// DefaultUseStatsd use statsd as a stats sink, default is true.
	DefaultUseStatsd = true
	// DefaultStatsdHost is the default address where statsd is running at.
	DefaultStatsdHost = "localhost"
	// DefaultStatsdPort is the default port where statsd is listening at.
	DefaultStatsdPort = 8125
	// DefaultFlushIntervalS is the default flushing interval in seconds.
	DefaultFlushIntervalS = 5
)

// The Settings type is used to configure gostats. gostats uses environment
// variables to setup its settings.
type Settings struct {
	// Use statsd as a stats sink.
	UseStatsd bool `envconfig:"USE_STATSD" default:"true"`
	// Address where statsd is running at.
	StatsdHost string `envconfig:"STATSD_HOST" default:"localhost"`
	// Port where statsd is listening at.
	StatsdPort int `envconfig:"STATSD_PORT" default:"8125"`
	// Flushing interval.
	FlushIntervalS int `envconfig:"GOSTATS_FLUSH_INTERVAL_SECONDS" default:"5"`
}

// An envError is an error that occured parsing an environment variable
type envError struct {
	Key   string
	Value string
	Err   error
}

func (e *envError) Error() string {
	return fmt.Sprintf("parsing environment variable: %q with value: %q: %s",
		e.Key, e.Value, e.Err)
}

func envOr(key, def string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return def
}

func envInt(key string, def int) (int, error) {
	s := os.Getenv(key)
	if s == "" {
		return def, nil
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return def, &envError{Key: key, Value: s, Err: err}
	}
	return i, nil
}

func envBool(key string, def bool) (bool, error) {
	s := os.Getenv(key)
	if s == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return def, &envError{Key: key, Value: s, Err: err}
	}
	return b, nil
}

// GetSettings returns the Settings gostats will run with.
func GetSettings() Settings {
	useStatsd, err := envBool("USE_STATSD", DefaultUseStatsd)
	if err != nil {
		panic(err)
	}
	statsdPort, err := envInt("STATSD_PORT", DefaultStatsdPort)
	if err != nil {
		panic(err)
	}
	flushIntervalS, err := envInt("GOSTATS_FLUSH_INTERVAL_SECONDS", DefaultFlushIntervalS)
	if err != nil {
		panic(err)
	}
	return Settings{
		UseStatsd:      useStatsd,
		StatsdHost:     envOr("STATSD_HOST", DefaultStatsdHost),
		StatsdPort:     statsdPort,
		FlushIntervalS: flushIntervalS,
	}
}
