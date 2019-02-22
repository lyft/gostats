package mock_test

import (
	"github.com/lyft/gostats"
	"github.com/lyft/gostats/mock"
)

var _ stats.Sink = (*mock.Sink)(nil)
var _ stats.FlushableSink = (*mock.Sink)(nil)
