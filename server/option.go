package server

const (
	DefaultGrpcMaxRecvSize = 200 << 20
)

type config struct {
	grpcMaxRecvSize int
	ca, cert, key   []byte
	dev             bool
}

func defaultOption() *config {
	return &config{
		grpcMaxRecvSize: DefaultGrpcMaxRecvSize,
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
		if size > 0 {
			c.grpcMaxRecvSize = size
		}
	})
}

func WithCredentials(cert, key []byte) Option {
	return newImplOption(func(c *config) {
		c.cert = cert
		c.key = key
	})
}

func WithCertificateAuthority(ca []byte) Option {
	return newImplOption(func(c *config) {
		c.ca = ca
	})
}

func WithDevMode(enable bool) Option {
	return newImplOption(func(c *config) {
		c.dev = enable
	})
}
