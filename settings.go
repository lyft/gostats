package stats

import "github.com/kelseyhightower/envconfig"

type Settings struct {
	UseStatsd           bool   `envconfig:"USE_STATSD" default:"true"`
	StatsdHost          string `envconfig:"STATSD_HOST" default:"localhost"`
	StatsdPort          int    `envconfig:"STATSD_PORT" default:"8125"`
	FlushIntervalS      int    `envconfig:"GOSTATS_FLUSH_INTERVAL_SECONDS" default:"5"`
}

func GetSettings() Settings {
	var s Settings
	err := envconfig.Process("", &s)
	if err != nil {
		panic(err)
	}
	return s
}