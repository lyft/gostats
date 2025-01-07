package stats

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

const (
	// DefaultUseStatsd use statsd as a stats sink, default is true.
	DefaultUseStatsd = true
	// DefaultStatsdHost is the default address where statsd is running at.
	DefaultStatsdHost = "localhost"
	// DefaultStatsdProtocol is TCP
	DefaultStatsdProtocol = "tcp"
	// DefaultStatsdPort is the default port where statsd is listening at.
	DefaultStatsdPort = 8125
	// DefaultFlushIntervalS is the default flushing interval in seconds.
	DefaultFlushIntervalS = 5
	// DefaultLoggingSinkDisabled is the default behavior of logging sink suppression, default is false.
	DefaultLoggingSinkDisabled = false
)

// The Settings type is used to configure gostats. gostats uses environment
// variables to setup its settings.
type Settings struct {
	// Use statsd as a stats sink.
	UseStatsd bool `mapstructure:"USE_STATSD"`
	// Address where statsd is running at.
	StatsdHost string `mapstructure:"STATSD_HOST"`
	// Network protocol used to connect to statsd
	StatsdProtocol string `mapstructure:"STATSD_PROTOCOL"`
	// Port where statsd is listening at.
	StatsdPort int `mapstructure:"STATSD_PORT"`
	// Flushing interval.
	FlushIntervalS int `mapstructure:"GOSTATS_FLUSH_INTERVAL_SECONDS"`
	// Disable the LoggingSink when USE_STATSD is false and use the NullSink instead.
	// This will cause all stats to be silently dropped.
	LoggingSinkDisabled bool `mapstructure:"GOSTATS_LOGGING_SINK_DISABLED"`
}

// An envError is an error that occurred parsing an environment variable
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
	viper.SetDefault("USE_STATSD", DefaultUseStatsd)
	viper.SetDefault("STATSD_HOST", DefaultStatsdHost)
	viper.SetDefault("STATSD_PROTOCOL", DefaultStatsdProtocol)
	viper.SetDefault("STATSD_PORT", DefaultStatsdPort)
	viper.SetDefault("GOSTATS_FLUSH_INTERVAL_SECONDS", DefaultFlushIntervalS)
	viper.SetDefault("GOSTATS_LOGGING_SINK_DISABLED", DefaultLoggingSinkDisabled)

	viper.AutomaticEnv()

	var settings Settings
	if err := viper.Unmarshal(&settings); err != nil {
		panic(fmt.Errorf("unable to decode into struct, %v", err))
	}

	return settings
}

// FlushInterval returns the flush interval duration.
func (s *Settings) FlushInterval() time.Duration {
	return time.Duration(s.FlushIntervalS) * time.Second
}
