package fluent

import (
	"crypto/rsa"
	"crypto"
	"crypto/sha512"
	"io/ioutil"
	"github.com/pkg/errors"
	"encoding/pem"
	"crypto/x509"
	"hash"
	"github.com/streadway/amqp"
)

const hashAlgo = x509.SHA512WithRSA
const signerAlgo = crypto.SHA512

func getHasher() hash.Hash {
	return sha512.New()
}

func sign(data []byte, key *rsa.PrivateKey, opts crypto.SignerOpts) ([]byte, error) {
	h := getHasher()
	h.Write(data)
	d := h.Sum(nil)
	return key.Sign(nil, d, opts)
}

func loadPrivateKeyFile(path string) (*rsa.PrivateKey, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "load content of "+path)
	}
	key, err := loadPrivateKey(content)
	return key, errors.Wrapf(err, "private key from file %v", path)
}

func loadPrivateKey(content []byte) (*rsa.PrivateKey, error) {
	var block *pem.Block
	tail := content
	for {
		block, tail = pem.Decode(tail)
		if block == nil {
			break
		}
		switch block.Type {
		case "PRIVATE KEY":
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, errors.Wrapf(err, "decode private key")
			}
			rKey, ok := key.(*rsa.PrivateKey)
			if !ok {
				return nil, errors.Errorf("private key is not RSA PKS#8")
			}
			return rKey, nil
		}
	}
	return nil, errors.New("private key not found")
}

func loadCertificateFromFile(path string) (*x509.Certificate, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "load content of "+path)
	}
	cert, err := loadCertificate(content)
	return cert, errors.Wrapf(err, "load certificate from file %v", path)
}
func loadCertificate(content []byte) (*x509.Certificate, error) {
	var block *pem.Block
	tail := content
	for {
		block, tail = pem.Decode(tail)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			key, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, errors.Wrapf(err, "decode public key")
			}
			return key, nil
		}
	}
	return nil, errors.New("public key not found")
}

type messageSigner struct {
	header string
	key    *rsa.PrivateKey
}

func (ms *messageSigner) Handle(msg *amqp.Publishing) bool {
	idBytes := []byte(msg.MessageId)
	var data = make([]byte, len(idBytes)+len(msg.Body))
	// by adding message-id into signature we prevent re-use old message by hackers. Of course only if target system
	// can correct handle duplicated messages (drop them)
	copy(data, idBytes)
	copy(data[len(idBytes):], msg.Body)

	signature, err := sign(data, ms.key, signerAlgo)
	if err != nil {
		panic(err) // something really wrong
	}
	if msg.Headers == nil {
		msg.Headers = make(amqp.Table)
	}
	msg.Headers[ms.header] = signature
	return true
}

func NewSigner(privateKey []byte, header string) (SenderHandler, error) {
	key, err := loadPrivateKey(privateKey)
	return &messageSigner{header: header, key: key}, errors.Wrapf(err, "create signer")
}

func NewSignerFromFile(privateKeyFile string, header string) (SenderHandler, error) {
	key, err := loadPrivateKeyFile(privateKeyFile)
	return &messageSigner{header: header, key: key}, errors.Wrapf(err, "create signer from file")
}

type messageValidator struct {
	logger Logger
	header string
	cert   *x509.Certificate
}

func (mv *messageValidator) Handle(msg *amqp.Delivery) (bool) {
	if msg.Headers == nil {
		mv.logger.Println("message", msg.MessageId, "has empty headers")
		return false
	}
	sigRaw, ok := msg.Headers[mv.header]
	if !ok {
		mv.logger.Println("message", msg.MessageId, "has no signature header", mv.header)
		return false
	}
	signature, ok := sigRaw.([]byte)
	if !ok {
		mv.logger.Println("message", msg.MessageId, "signature header is not a bytes")
		return false
	}

	idBytes := []byte(msg.MessageId)
	var data = make([]byte, len(idBytes)+len(msg.Body))
	copy(data, idBytes)
	copy(data[len(idBytes):], msg.Body)

	err := mv.cert.CheckSignature(hashAlgo, data, signature)
	if err != nil {
		mv.logger.Println("message", msg.MessageId, "signature verification failed:", err)
		return false
	}
	return true
}

func NewCertValidator(cert []byte, header string, log Logger) (ReceiverHandler, error) {
	key, err := loadCertificate(cert)
	return &messageValidator{header: header, cert: key, logger: log}, errors.Wrapf(err, "create validator")
}

func NewCertValidatorFromFile(certFile string, header string, log Logger) (ReceiverHandler, error) {
	key, err := loadCertificateFromFile(certFile)
	return &messageValidator{header: header, cert: key, logger: log}, errors.Wrapf(err, "create validator from file")
}
