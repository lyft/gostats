package bufferpool

import "github.com/lyft/gostats/internal/buffer"

var pool = buffer.NewPool()

func Get() *buffer.Buffer { return pool.Get() }
