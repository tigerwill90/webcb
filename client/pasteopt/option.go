package pasteopt

import "github.com/tigerwill90/webcb/server"

type Option interface {
	Apply(*Config)
}

type Config struct {
	ChunkSize int64
}

func DefaultConfig() *Config {
	return &Config{
		ChunkSize: server.DefaultTransferRate,
	}
}

type implPasteOption struct {
	f func(*Config)
}

func (o *implPasteOption) Apply(c *Config) {
	o.f(c)
}

func newImplPasteOption(f func(*Config)) *implPasteOption {
	return &implPasteOption{f: f}
}

func WithTransferRate(n int64) *implPasteOption {
	return newImplPasteOption(func(c *Config) {
		if n > 0 {
			c.ChunkSize = n
		}
	})
}
