package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
)

func LoadRootCa(ca []byte) (*x509.CertPool, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca) {
		return nil, errors.New("unable to append root certificate to the ca pool")
	}
	return caPool, nil
}

func LoadKeyPair(cert, key []byte) (tls.Certificate, error) {
	c, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return c, nil
}

func LoadCertificate(ca, cert, key []byte, config *tls.Config) error {
	var rootCAs *x509.CertPool
	if len(ca) != 0 {
		var err error
		rootCAs, err = LoadRootCa(ca)
		if err != nil {
			return err
		}
	}

	if len(cert) == 0 || len(key) == 0 {
		return errors.New("both client certificate and key are required")
	}

	certificate, err := LoadKeyPair(cert, key)
	if err != nil {
		return err
	}

	config.Certificates = append(config.Certificates, certificate)
	config.RootCAs = rootCAs
	config.ClientCAs = rootCAs
	return nil
}
