package client

import (
	"github.com/tigerwill90/webcb/server"
	"time"
)

type config struct {
	chunkSize   int64
	checksum    bool
	ttl         time.Duration
	compression bool
	password    []byte
}

func defaultConfig() *config {
	return &config{
		chunkSize: 1 * 1024 * 1024,
		ttl:       server.DefaultTtl,
	}
}

type Option func(*config)

func WithTransferRate(n int64) Option {
	return func(c *config) {
		if n > 0 {
			c.chunkSize = n
		}
	}
}

func WithChecksum(enable bool) Option {
	return func(c *config) {
		c.checksum = enable
	}
}

func WithTtl(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.ttl = d
		}
	}
}

func WithCompression(enable bool) Option {
	return func(c *config) {
		c.compression = enable
	}
}

func WithPassword(password string) Option {
	return func(c *config) {
		c.password = []byte(password)
	}
}
