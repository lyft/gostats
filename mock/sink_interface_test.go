package mock_test

import (
	stats "github.com/lyft/gostats"
	"github.com/lyft/gostats/mock"
)

var (
	_ stats.Sink          = (*mock.Sink)(nil)
	_ stats.FlushableSink = (*mock.Sink)(nil)
)
