package server

type config struct {
	grpcMaxRecvSize int
}

func defaultOption() *config {
	return &config{
		grpcMaxRecvSize: 200 * 1024 * 1024,
	}
}

type Option interface {
	apply(*config)
}

type implOption struct {
	f func(*config)
}

func (o *implOption) apply(c *config) {
	o.f(c)
}

func newImplOption(f func(*config)) *implOption {
	return &implOption{f: f}
}

func WithGrpcMaxRecvSize(size int) Option {
	return newImplOption(func(c *config) {
		c.grpcMaxRecvSize = size
	})
}
