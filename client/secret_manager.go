package client

type SecretManager interface {
	Read() ([]byte, error)
}

type Password func() ([]byte, error)

func (p Password) Read() ([]byte, error) {
	return p()
}
