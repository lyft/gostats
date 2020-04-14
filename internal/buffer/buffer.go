package buffer

import (
	"strconv"
	"sync"
)

const initialSize = 256

// Use a fast and simple buffer for constructing statsd messages
type Buffer struct {
	bs   []byte
	pool Pool
}

func (b *Buffer) Len() int { return len(b.bs) }

func (b *Buffer) Reset() { b.bs = b.bs[:0] }

func (b *Buffer) Bytes() []byte { return b.bs }

func (b *Buffer) String() string { return string(b.bs) }

func (b *Buffer) Write(p []byte) (int, error) {
	b.bs = append(b.bs, p...)
	return len(p), nil
}

func (b *Buffer) WriteString(s string) {
	b.bs = append(b.bs, s...)
}

// This is named WriteChar instead of WriteByte because the 'stdmethods' check
// of 'go vet' wants WriteByte to have the signature:
//
// 	func (b *Buffer) WriteByte(c byte) error { ... }
//
func (b *Buffer) WriteChar(c byte) {
	b.bs = append(b.bs, c)
}

func (b *Buffer) WriteUnit64(val uint64) {
	b.bs = strconv.AppendUint(b.bs, val, 10)
}

func (b *Buffer) WriteFloat64(val float64) {
	b.bs = strconv.AppendFloat(b.bs, val, 'f', 6, 64)
}

func (b *Buffer) Free() {
	b.pool.put(b)
}

type Pool struct {
	p *sync.Pool
}

func NewPool() Pool {
	return Pool{
		p: &sync.Pool{
			New: func() interface{} {
				return &Buffer{bs: make([]byte, 0, initialSize)}
			},
		},
	}
}

func (p Pool) Get() *Buffer {
	buf := p.p.Get().(*Buffer)
	buf.Reset()
	buf.pool = p
	return buf
}

func (p *Pool) put(buf *Buffer) {
	p.p.Put(buf)
}
