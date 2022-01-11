package crypto

import (
	"crypto/sha256"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/scrypt"
	"io"
)

const (
	NonceSize = 32
)

func DerivePassword(password, nonce []byte) ([]byte, error) {
	key, err := scrypt.Key(password, nonce, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func DeriveMasterKey(masterKey, nonce []byte) ([]byte, error) {
	var key [32]byte
	kdf := hkdf.New(sha256.New, masterKey, nonce, nil)
	if _, err := io.ReadFull(kdf, key[:]); err != nil {
		return nil, err
	}
	return key[:], nil
}
