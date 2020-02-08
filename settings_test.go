package stats

import (
	"os"
	"reflect"
	"testing"

	"github.com/kelseyhightower/envconfig"
)

func testSetenv(t *testing.T, pairs ...string) (reset func()) {
	var fns []func()
	for i := 0; i < len(pairs); i += 2 {
		key := pairs[i+0]
		val := pairs[i+1]

		prev, exists := os.LookupEnv(key)
		if val == "" {
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("deleting env key: %s: %s", key, err)
			}
		} else {
			if err := os.Setenv(key, val); err != nil {
				t.Fatalf("setting env key: %s: %s", key, err)
			}
		}
		if exists {
			fns = append(fns, func() { os.Setenv(key, prev) })
		} else {
			fns = append(fns, func() { os.Unsetenv(key) })
		}
	}
	return func() {
		for _, fn := range fns {
			fn()
		}
	}
}

func TestSettingsCompat(t *testing.T) {
	reset := testSetenv(t,
		"USE_STATSD", "",
		"STATSD_HOST", "",
		"STATSD_PORT", "",
		"GOSTATS_FLUSH_INTERVAL_SECONDS", "",
	)
	defer reset()

	var e Settings
	if err := envconfig.Process("", &e); err != nil {
		t.Fatal(err)
	}

	s := GetSettings()
	if !reflect.DeepEqual(e, s) {
		t.Fatalf("Default Settings: want: %+v got: %+v", e, s)
	}
}

func TestSettingsDefault(t *testing.T) {
	reset := testSetenv(t,
		"USE_STATSD", "",
		"STATSD_HOST", "",
		"STATSD_PORT", "",
		"GOSTATS_FLUSH_INTERVAL_SECONDS", "",
	)
	defer reset()
	exp := Settings{
		UseStatsd:      DefaultUseStatsd,
		StatsdHost:     DefaultStatsdHost,
		StatsdPort:     DefaultStatsdPort,
		FlushIntervalS: DefaultFlushIntervalS,
	}
	settings := GetSettings()
	if exp != settings {
		t.Errorf("Default: want: %+v got: %+v", exp, settings)
	}
}

func TestSettingsOverride(t *testing.T) {
	reset := testSetenv(t,
		"USE_STATSD", "true",
		"STATSD_HOST", "10.0.0.1",
		"STATSD_PORT", "1234",
		"GOSTATS_FLUSH_INTERVAL_SECONDS", "3",
	)
	defer reset()
	exp := Settings{
		UseStatsd:      true,
		StatsdHost:     "10.0.0.1",
		StatsdPort:     1234,
		FlushIntervalS: 3,
	}
	settings := GetSettings()
	if exp != settings {
		t.Errorf("Default: want: %+v got: %+v", exp, settings)
	}
}

func TestSettingsErrors(t *testing.T) {
	// STATSD_HOST doesn't error so we don't check it

	tests := map[string]string{
		"USE_STATSD":                     "FOO!",
		"STATSD_PORT":                    "not-an-int",
		"GOSTATS_FLUSH_INTERVAL_SECONDS": "true",
	}
	for key, val := range tests {
		t.Run(key, func(t *testing.T) {
			reset := testSetenv(t, key, val)
			defer reset()
			var panicked bool
			func() {
				defer func() {
					panicked = recover() != nil
				}()
				GetSettings()
			}()
			if !panicked {
				t.Errorf("Settings expected a panic for invalid value %s=%s", key, val)
			}
		})
	}
}
