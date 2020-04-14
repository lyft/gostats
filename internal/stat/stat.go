package stat

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/lyft/gostats/internal/buffer"
	"github.com/lyft/gostats/internal/bufferpool"
)

type StatType uint32

const (
	CounterStat = StatType(iota + 1)
	GaugeStat
	TimerStat
)

var ErrInvalidStatType = errors.New("invalid stat type")

func (s StatType) Valid() error {
	if CounterStat <= s && s <= TimerStat {
		return nil
	}
	return ErrInvalidStatType
}

func (s StatType) String() string {
	switch s {
	case CounterStat:
		return "Counter"
	case GaugeStat:
		return "Gauge"
	case TimerStat:
		return "Timer"
	default:
		return "StatType(" + strconv.FormatUint(uint64(s), 10) + ")"
	}
}

// TODO: add WriteTo() method
type Stat struct {
	Type  StatType // TODO: make first struct field
	Name  string
	Value uint64
}

func (s Stat) valid() bool {
	return CounterStat <= s.Type && s.Type <= TimerStat
}

func (s Stat) String() string {
	if s.Type == TimerStat {
		return fmt.Sprintf("{Type: %s Name: %q Value: %f}",
			s.Type, s.Name, math.Float64frombits(s.Value))
	}
	return fmt.Sprintf("{Type: %s Name: %q Value: %d}",
		s.Type, s.Name, s.Value)
}

func appendFloat64(b *buffer.Buffer, value float64) {
	// MaxInt is the largest integer that can be stored in a double
	// precision float without losing precision.
	const MaxInt = 1 << 53

	// TODO: can a timer ever be negative?
	if 0 <= value && value <= MaxInt && math.Trunc(value) == value {
		b.WriteUnit64(uint64(value))
	} else {
		b.WriteFloat64(value)
	}
}

func (s *Stat) Format(b *buffer.Buffer) {
	b.WriteString(s.Name)
	b.WriteChar(':')

	if s.Type == CounterStat {
		b.WriteUnit64(s.Value)
		b.WriteString("|c\n")
	} else if s.Type == GaugeStat {
		b.WriteUnit64(s.Value)
		b.WriteString("|g\n")
	} else {
		appendFloat64(b, math.Float64frombits(s.Value))
		b.WriteString("|ms\n")
	}
}

func (s *Stat) WriteTo(w io.Writer) (int64, error) {
	if !s.valid() {
		return 0, ErrInvalidStatType
	}

	b := bufferpool.Get()
	s.Format(b)
	n, err := w.Write(b.Bytes())
	b.Free()

	return int64(n), err
}
