package client

import "github.com/tigerwill90/webcb/server"

type config struct {
	chunkSize int64
	integrity bool
}

func defaultConfig() *config {
	return &config{
		chunkSize: server.DefaultChunkSize,
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
