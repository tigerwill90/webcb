package crypto

import "golang.org/x/crypto/scrypt"

const (
	SaltSize = 32
)

func DeriveKey(password, salt []byte) ([]byte, error) {
	key, err := scrypt.Key(password, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	return key, nil
}
