package copyopt

import (
	"github.com/tigerwill90/webcb/server"
	"time"
)

type Option interface {
	Apply(*Config)
}

type Config struct {
	ChunkSize   int64
	Checksum    bool
	Ttl         time.Duration
	Compression bool
	Password    []byte
}

func DefaultConfig() *Config {
	return &Config{
		ChunkSize: server.DefaultTransferRate,
		Ttl:       server.DefaultTtl,
	}
}

type implCopyOption struct {
	f func(*Config)
}

func (o *implCopyOption) Apply(c *Config) {
	o.f(c)
}

func newImplCopyOption(f func(c *Config)) *implCopyOption {
	return &implCopyOption{f: f}
}

func WithTransferRate(n int64) *implCopyOption {
	return newImplCopyOption(func(c *Config) {
		if n > 0 {
			c.ChunkSize = n
		}
	})
}

func WithChecksum(enable bool) *implCopyOption {
	return newImplCopyOption(func(c *Config) {
		c.Checksum = enable
	})
}

func WithTtl(d time.Duration) *implCopyOption {
	return newImplCopyOption(func(c *Config) {
		if d > 0 {
			c.Ttl = d
		}
	})
}

func WithCompression(enable bool) *implCopyOption {
	return newImplCopyOption(func(c *Config) {
		c.Compression = enable
	})
}

func WithPassword(password string) *implCopyOption {
	return newImplCopyOption(func(c *Config) {
		c.Password = []byte(password)
	})
}
