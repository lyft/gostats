package stats

import "github.com/kelseyhightower/envconfig"

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

// GetSettings returns the Settings gostats will run with.
func GetSettings() Settings {
	var s Settings
	err := envconfig.Process("", &s)
	if err != nil {
		panic(err)
	}
	return s
}
