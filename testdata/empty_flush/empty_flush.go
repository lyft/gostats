package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	stats "github.com/lyft/gostats"
)

const FlushTimeout = time.Second * 3

func flushStore(store stats.Store) error {
	done := make(chan struct{})
	go func() { store.Flush(); close(done) }()
	select {
	case <-done:
		return nil
	case <-time.After(FlushTimeout):
		return errors.New("stats flush timed out")
	}
}

func realMain() (err error) {
	store := stats.NewDefaultStore()

	scope := store.ScopeWithTags("test.service.name", map[string]string{
		"wrap": "1",
	})
	defer func() {
		err = flushStore(store)
	}()
	_ = scope.NewCounter("panics")
	return
}

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
