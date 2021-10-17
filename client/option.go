package client

import (
	"github.com/tigerwill90/webcb/server"
	"time"
)

type config struct {
	chunkSize int64
	integrity bool
	ttl       time.Duration
}

func defaultConfig() *config {
	return &config{
		chunkSize: 1 * 1024 * 1024,
		ttl:       server.DefaultTtl,
	}
}

type Option func(*config)

func WithChunkSize(n int64) Option {
	return func(c *config) {
		if n > 0 {
			c.chunkSize = n
		}
	}
}

func WithIntegrity(enable bool) Option {
	return func(c *config) {
		c.integrity = enable
	}
}

func WithTtl(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.ttl = d
		}
	}
}
