package stats

import (
	"errors"
	"fmt"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lyft/gostats/internal/tags"
	"github.com/lyft/gostats/mock"
)

type (
	ignoreErrors testing.T
	logErrors    testing.T
)

var _ testing.TB = (*ignoreErrors)(nil)
var _ testing.TB = (*logErrors)(nil)

// Pass IgnoreErrors to NewStore to ignore stats errors.
var (
	IgnoreErrors testing.TB = new(ignoreErrors)
	LogErrors    testing.TB = new(logErrors)
)

var _ Store = (*MockStore)(nil)
var _ Scope = (*MockStore)(nil)

type MockStore struct {
	store *statStore
	sink  *mock.Sink
	t     testing.TB
}

func NewMockStore(t testing.TB) (*MockStore, *mock.Sink) {
	sink := mock.NewSink()
	store := &MockStore{
		store: NewStore(sink, false).(*statStore),
		sink:  sink,
		t:     t,
	}
	return store, sink
}

// Reset resets the underlying mock.Sink to a fresh state.
func (s *MockStore) Reset() { s.sink.Reset() }

// Sink returns the underlying mock.Sink.
func (s *MockStore) Sink() *mock.Sink { return s.sink }

func (s *MockStore) Flush() { s.store.Flush() }

func (s *MockStore) Start(_ *time.Ticker) { /* no-op we flush on every write */ }

func (s *MockStore) AddStatGenerator(sg StatGenerator) {
	s.store.AddStatGenerator(sg)
}

func (s *MockStore) Scope(name string) Scope {
	s.validateStats(name, nil)
	v := s.store.Scope(name)
	s.Flush()
	return v
}

func (s *MockStore) ScopeWithTags(name string, tags map[string]string) Scope {
	s.validateStats(name, tags)
	v := s.store.ScopeWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) Store() Store { return s.store.Store() }

func (s *MockStore) NewCounter(name string) Counter {
	s.validateStats(name, nil)
	v := s.store.NewCounter(name)
	s.Flush()
	return v
}

func (s *MockStore) NewCounterWithTags(name string, tags map[string]string) Counter {
	s.validateStats(name, tags)
	v := s.store.NewCounterWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	s.validateStats(name, tags)
	v := s.store.NewPerInstanceCounter(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewGauge(name string) Gauge {
	s.validateStats(name, nil)
	v := s.store.NewGauge(name)
	s.Flush()
	return v
}

func (s *MockStore) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	s.validateStats(name, tags)
	v := s.store.NewGaugeWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	s.validateStats(name, tags)
	v := s.store.NewPerInstanceGauge(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewTimer(name string) Timer {
	s.validateStats(name, nil)
	v := s.store.NewTimer(name)
	s.Flush()
	return v
}

func (s *MockStore) NewTimerWithTags(name string, tags map[string]string) Timer {
	s.validateStats(name, tags)
	v := s.store.NewTimerWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	s.validateStats(name, tags)
	v := s.store.NewPerInstanceTimer(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) errorf(format string, args ...interface{}) {
	switch s.t {
	case IgnoreErrors:
		// no-op
	case LogErrors:
		s.t.Logf(format, args...)
	case nil:
		panic(fmt.Sprintf(format, args...))
	default:
		s.t.Errorf(format, args...)
	}
}

func validateStat(s string) error {
	if s == "" {
		return errors.New("empty string")
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("invalid UTF8: %q", s)
	}
	for _, r := range s {
		if r > utf8.RuneSelf {
			return fmt.Errorf("contains non-ASCII characters: %q", s)
		}
		if !unicode.IsPrint(r) {
			return fmt.Errorf("contains non-Printable characters: %q", s)
		}
	}
	return nil
}

func (s *MockStore) validateStats(name string, m map[string]string) {
	if err := validateStat(name); err != nil {
		s.errorf("stat name: %s", err)
	}
	for k, v := range m {
		if err := validateStat(k); err != nil {
			s.errorf("tag key: %s", err)
		}
		if err := validateStat(v); err != nil {
			s.errorf("tag value: %s", err)
		}
		if clean := tags.ReplaceChars(v); clean != v {
			s.errorf("tag value: invalid chars: %q vs. %q", v, clean)
		}
	}
}
