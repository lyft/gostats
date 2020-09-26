package stats

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	SocketReuseTimeout = 5 * time.Second
)

type TestConn interface {
	Close() (err error)
	Address() net.Addr
	Reconnect(t testing.TB, s *netTestSink)
	Run(t testing.TB)
}

type udpConn struct {
	ll        *net.UDPConn
	addr      *net.UDPAddr
	writeStat func(line []byte)
	done      chan struct{}
}

func NewUDPConn(t testing.TB, s *netTestSink) TestConn {
	l, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal("ListenUDP:", err)
	}
	c := &udpConn{
		ll:        l,
		addr:      l.LocalAddr().(*net.UDPAddr),
		writeStat: s.writeStat,
		done:      s.done,
	}
	go c.Run(t)
	return c
}

func (c *udpConn) Address() net.Addr {
	return c.ll.LocalAddr()
}

func (c *udpConn) Close() (err error) {
	return c.ll.Close()
}

func (c *udpConn) Reconnect(t testing.TB, s *netTestSink) {
	reconnectRetry(t, func() error {
		l, err := net.ListenUDP(c.addr.Network(), c.addr)
		if err != nil {
			return err
		}
		c.ll = l
		c.writeStat = s.writeStat
		c.done = s.done
		go c.Run(t)
		return nil
	})
}

func (c *udpConn) Run(t testing.TB) {
	defer close(c.done)
	buf := bufio.NewReader(c.ll)
	var err error
	for {
		b, e := buf.ReadBytes('\n')
		if len(b) > 0 {
			c.writeStat(b)
		}
		if e != nil {
			if e != io.EOF {
				err = e
			}
			break
		}
	}
	if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		t.Logf("Error: reading stats: %v", err)
	}
}

type tcpConn struct {
	ll        *net.TCPListener
	addr      *net.TCPAddr
	writeStat func(line []byte)
	done      chan struct{}
}

func NewTCPConn(t testing.TB, s *netTestSink) TestConn {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal("ListenTCP:", err)
	}
	c := &tcpConn{
		ll:        l,
		addr:      l.Addr().(*net.TCPAddr),
		writeStat: s.writeStat,
		done:      s.done,
	}
	go c.Run(t)
	return c
}

func (c *tcpConn) Address() net.Addr {
	return c.ll.Addr()
}

func (c *tcpConn) Close() (err error) {
	return c.ll.Close()
}

func (c *tcpConn) Reconnect(t testing.TB, s *netTestSink) {
	reconnectRetry(t, func() error {
		l, err := net.ListenTCP(c.addr.Network(), c.addr)
		if err != nil {
			return err
		}
		c.ll = l
		c.writeStat = s.writeStat
		c.done = s.done
		go c.Run(t)
		return nil
	})
}

func (c *tcpConn) Run(t testing.TB) {
	defer close(c.done)
	buf := bufio.NewReader(nil)
	for {
		conn, err := c.ll.AcceptTCP()
		if err != nil {
			// Log errors other than poll.ErrNetClosing, which is an
			// internal error so we have to match against it's string.
			if !strings.Contains(err.Error(), "use of closed network connection") {
				t.Logf("Error: accept: %v", err)
			}
			return
		}
		// read stats line by line
		buf.Reset(conn)
		for {
			b, e := buf.ReadBytes('\n')
			if len(b) > 0 {
				c.writeStat(b)
			}
			if e != nil {
				if e != io.EOF {
					err = e
				}
				break
			}
		}
		if err != nil {
			t.Errorf("Error: reading stats: %v", err)
		}
	}
}

type netTestSink struct {
	conn     TestConn
	mu       sync.Mutex // buf lock
	buf      bytes.Buffer
	stats    chan string
	done     chan struct{} // closed when read loop exits
	protocol string
}

func newNetTestSink(t testing.TB, protocol string) *netTestSink {
	s := &netTestSink{
		stats:    make(chan string, 64),
		done:     make(chan struct{}),
		protocol: protocol,
	}
	switch protocol {
	case "udp":
		s.conn = NewUDPConn(t, s)
	case "tcp":
		s.conn = NewTCPConn(t, s)
	default:
		t.Fatalf("invalid network protocol: %q", protocol)
	}
	return s
}

func (s *netTestSink) writeStat(line []byte) {
	select {
	case s.stats <- string(line):
	default:
	}
	s.mu.Lock()
	s.buf.Write(line)
	s.mu.Unlock()
}

func (s *netTestSink) Restart(t testing.TB, resetBuffer bool) {
	if err := s.Close(); err != nil {
		if !strings.Contains(err.Error(), "use of closed network connection") {
			t.Fatal(err)
		}
	}
	select {
	case <-s.done:
		// Ok
	case <-time.After(time.Second * 3):
		t.Fatal("timeout waiting for run loop to exit")
	}

	if resetBuffer {
		s.buf.Reset()
	}
	s.stats = make(chan string, 64)
	s.done = make(chan struct{})
	s.conn.Reconnect(t, s)
}

func (s *netTestSink) WaitForStat(t testing.TB, timeout time.Duration) string {
	t.Helper()
	if timeout <= 0 {
		timeout = defaultRetryInterval * 2
	}
	to := time.NewTimer(timeout)
	defer to.Stop()
	select {
	case s := <-s.stats:
		return s
	case <-to.C:
		t.Fatalf("timeout waiting to receive stat: %s", timeout)
	}
	return ""
}

func (s *netTestSink) Close() error {
	select {
	case <-s.done:
		return nil // closed
	default:
		return s.conn.Close() // WARN
		// return s.ll.Close() // WARN
	}
}

func (s *netTestSink) Stats() <-chan string {
	return s.stats
}

func (s *netTestSink) Bytes() []byte {
	s.mu.Lock()
	b := append([]byte(nil), s.buf.Bytes()...)
	s.mu.Unlock()
	return b
}

func (s *netTestSink) String() string {
	s.mu.Lock()
	str := s.buf.String()
	s.mu.Unlock()
	return str
}

func (s *netTestSink) Host(t testing.TB) string {
	t.Helper()
	host, _, err := net.SplitHostPort(s.conn.Address().String())
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func (s *netTestSink) Port(t testing.TB) int {
	t.Helper()
	_, port, err := net.SplitHostPort(s.conn.Address().String())
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (s *netTestSink) Protocol() string {
	return s.protocol
}

func mergeEnv(extra ...string) []string {
	var prefixes []string
	for _, s := range extra {
		n := strings.IndexByte(s, '=')
		prefixes = append(prefixes, s[:n+1])
	}
	ignore := func(s string) bool {
		for _, pfx := range prefixes {
			if strings.HasPrefix(s, pfx) {
				return true
			}
		}
		return false
	}

	env := os.Environ()
	a := env[:0]
	for _, s := range env {
		if !ignore(s) {
			a = append(a, s)
		}
	}
	return append(a, extra...)
}

func (s *netTestSink) CommandEnv(t testing.TB) []string {
	return mergeEnv(
		fmt.Sprintf("STATSD_PORT=%d", s.Port(t)),
		fmt.Sprintf("STATSD_HOST=%s", s.Host(t)),
		fmt.Sprintf("STATSD_PROTOCOL=%s", s.Protocol()),
		"GOSTATS_FLUSH_INTERVAL_SECONDS=1",
	)
}

func reconnectRetry(t testing.TB, fn func() error) {
	const (
		Retry = time.Second / 4
		N     = int(SocketReuseTimeout / Retry)
	)
	var err error
	for i := 0; i < N; i++ {
		err = fn()
		if err == nil {
			return
		}
		// Retry if the error is due to the address being in use.
		// On slow systems (CI) it can take awhile for the OS to
		// realize the address is not in use.
		if errors.Is(err, syscall.EADDRINUSE) {
			time.Sleep(Retry)
		} else {
			t.Fatalf("unexpected error reconnecting: %s", err)
			return // unreachable
		}
	}
	t.Fatalf("failed to reconnect after %d attempts and %s: %v",
		N, SocketReuseTimeout, err)
}

func TestReconnectRetryTCP(t *testing.T) {
	t.Parallel()

	l1, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()

	l2, err := net.ListenTCP(l1.Addr().Network(), l1.Addr().(*net.TCPAddr))
	if err == nil {
		l2.Close()
		t.Fatal("expected an error got nil")
	}

	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("expected error to wrap %T got: %#v", syscall.EADDRINUSE, err)
	}

	first := true
	callCount := 0
	reconnectRetry(t, func() error {
		callCount++
		if first {
			first = false
			return err
		}
		return nil
	})

	if callCount != 2 {
		t.Errorf("Expected call cound to be %d got: %d", 2, callCount)
	}
}

func TestReconnectRetryUDP(t *testing.T) {
	t.Parallel()

	l1, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()

	l2, err := net.ListenUDP(l1.LocalAddr().Network(), l1.LocalAddr().(*net.UDPAddr))
	if err == nil {
		l2.Close()
		t.Fatal("expected an error got nil")
	}

	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("expected error to wrap %T got: %#v", syscall.EADDRINUSE, err)
	}

	first := true
	callCount := 0
	reconnectRetry(t, func() error {
		callCount++
		if first {
			first = false
			return err
		}
		return nil
	})

	if callCount != 2 {
		t.Errorf("Expected call cound to be %d got: %d", 2, callCount)
	}
}
