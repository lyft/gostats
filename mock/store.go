package mock

/*
import (
	"errors"
	"fmt"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	stats "github.com/lyft/gostats"
	"github.com/lyft/gostats/internal/tags"
)

var _ stats.Store = (*MockStore)(nil)

type MockStore struct {
	store stats.Store
	sink  *Sink
	t     testing.TB
}

// Pass IgnoreErrors to NewStore to ignore stats errors.
var IgnoreErrors testing.TB = new(testing.T)

type ignoreErrors *testing.T

func NewStore(t testing.TB) (*MockStore, *Sink) {
	sink := NewSink()
	store := &MockStore{
		store: stats.NewStore(sink, false),
		sink:  sink,
		t:     t,
	}
	return store, sink
}

// Sink returns the underlying mock.Sink.
func (s *MockStore) Sink() *Sink { return s.sink }

func (s *MockStore) Flush() { s.store.Flush() }

func (s *MockStore) Start(_ *time.Ticker) {
	// no-op we flush on every write
}

func (s *MockStore) AddStatGenerator(sg stats.StatGenerator) {
	s.store.AddStatGenerator(sg)
}

func (s *MockStore) Scope(name string) stats.Scope {
	v := s.store.Scope(name)
	s.Flush()
	return v
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

func (s *MockStore) errorf(format string, args ...interface{}) {
	switch s.t {
	case IgnoreErrors:
		// no-op
	case nil:
		panic(fmt.Sprintf(format, args...))
	default:
		s.t.Errorf(format, args...)
	}
}

func (s *MockStore) validateTags(m map[string]string) {
	if s.t == IgnoreErrors {
		return
	}
	if s.t == nil {
		return
	}
	for k, v := range m {
		if err := validateStat(k); err != nil {
			s.t.Errorf("tag key: %s", err)
		}
		if err := validateStat(v); err != nil {
			s.t.Errorf("tag value: %s", err)
		}
		if clean := tags.ReplaceChars(v); clean != v {
			s.t.Errorf("tag value: invalid chars: %q vs. %q", v, clean)
		}
	}
}

func (s *MockStore) ScopeWithTags(name string, tags map[string]string) stats.Scope {
	s.validateTags(tags)
	v := s.store.ScopeWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) Store() stats.Store {
	return s.store.Store()
}

func (s *MockStore) NewCounter(name string) stats.Counter {
	v := s.store.NewCounter(name)
	s.Flush()
	return v
}

func (s *MockStore) NewCounterWithTags(name string, tags map[string]string) stats.Counter {
	s.validateTags(tags)
	v := s.store.NewCounterWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewPerInstanceCounter(name string, tags map[string]string) stats.Counter {
	s.validateTags(tags)
	v := s.store.NewPerInstanceCounter(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewGauge(name string) stats.Gauge {
	v := s.store.NewGauge(name)
	s.Flush()
	return v
}

func (s *MockStore) NewGaugeWithTags(name string, tags map[string]string) stats.Gauge {
	s.validateTags(tags)
	v := s.store.NewGaugeWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewPerInstanceGauge(name string, tags map[string]string) stats.Gauge {
	s.validateTags(tags)
	v := s.store.NewPerInstanceGauge(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewTimer(name string) stats.Timer {
	v := s.store.NewTimer(name)
	s.Flush()
	return v
}

func (s *MockStore) NewTimerWithTags(name string, tags map[string]string) stats.Timer {
	s.validateTags(tags)
	v := s.store.NewTimerWithTags(name, tags)
	s.Flush()
	return v
}

func (s *MockStore) NewPerInstanceTimer(name string, tags map[string]string) stats.Timer {
	s.validateTags(tags)
	v := s.store.NewPerInstanceTimer(name, tags)
	s.Flush()
	return v
}
*/
