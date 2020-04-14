package stat

import (
	"fmt"
	"testing"

	"github.com/lyft/gostats/internal/bufferpool"
)

func TestStatTypeValid(t *testing.T) {
	var st StatType
	if err := st.Valid(); err == nil {
		t.Fatal("Zero value of StatType should be invalid")
	}
}

func BenchmarkFormat_Counter(b *testing.B) {
	const name = "TestCounter.___f=i.__tag1=v1"
	const value = 12345
	b.SetBytes(int64(len(fmt.Sprintf("%s:%d|n\n", name, value))))

	s := &Stat{
		Name:  name,
		Value: value,
		Type:  CounterStat,
	}
	buf := bufferpool.Get()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s.Format(buf)
		buf.Reset()
	}
	buf.Free()
}
