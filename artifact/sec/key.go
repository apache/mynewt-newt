/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package sec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"io/ioutil"

	keywrap "github.com/NickBall/go-aes-key-wrap"

	"mynewt.apache.org/newt/util"
)

type SignKey struct {
	// Only one of these members is non-nil.
	Rsa *rsa.PrivateKey
	Ec  *ecdsa.PrivateKey
}

func ParsePrivateKey(keyBytes []byte) (interface{}, error) {
	var privKey interface{}
	var err error

	block, data := pem.Decode(keyBytes)
	if block != nil && block.Type == "EC PARAMETERS" {
		/*
		 * Openssl prepends an EC PARAMETERS block before the
		 * key itself.  If we see this first, just skip it,
		 * and go on to the data block.
		 */
		block, _ = pem.Decode(data)
	}
	if block != nil && block.Type == "RSA PRIVATE KEY" {
		/*
		 * ParsePKCS1PrivateKey returns an RSA private key from its ASN.1
		 * PKCS#1 DER encoded form.
		 */
		privKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, util.FmtNewtError(
				"Private key parsing failed: %s", err)
		}
	}
	if block != nil && block.Type == "EC PRIVATE KEY" {
		/*
		 * ParseECPrivateKey returns a EC private key
		 */
		privKey, err = x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, util.FmtNewtError(
				"Private key parsing failed: %s", err)
		}
	}
	if block != nil && block.Type == "PRIVATE KEY" {
		// This indicates a PKCS#8 unencrypted private key.
		// The particular type of key will be indicated within
		// the key itself.
		privKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, util.FmtNewtError(
				"Private key parsing failed: %s", err)
		}
	}
	if block != nil && block.Type == "ENCRYPTED PRIVATE KEY" {
		// This indicates a PKCS#8 key wrapped with PKCS#5
		// encryption.
		privKey, err = parseEncryptedPrivateKey(block.Bytes)
		if err != nil {
			return nil, util.FmtNewtError(
				"Unable to decode encrypted private key: %s", err)
		}
	}
	if privKey == nil {
		return nil, util.NewNewtError(
			"Unknown private key format, EC/RSA private " +
				"key in PEM format only.")
	}

	return privKey, nil
}

func BuildPrivateKey(keyBytes []byte) (SignKey, error) {
	key := SignKey{}

	privKey, err := ParsePrivateKey(keyBytes)
	if err != nil {
		return key, err
	}

	switch priv := privKey.(type) {
	case *rsa.PrivateKey:
		key.Rsa = priv
	case *ecdsa.PrivateKey:
		key.Ec = priv
	default:
		return key, util.NewNewtError("Unknown private key format")
	}

	return key, nil
}

func ReadKey(filename string) (SignKey, error) {
	keyBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return SignKey{}, util.FmtNewtError(
			"Error reading key file: %s", err)
	}

	return BuildPrivateKey(keyBytes)
}

func ReadKeys(filenames []string) ([]SignKey, error) {
	keys := make([]SignKey, len(filenames))

	for i, filename := range filenames {
		key, err := ReadKey(filename)
		if err != nil {
			return nil, err
		}

		keys[i] = key
	}

	return keys, nil
}

func (key *SignKey) AssertValid() {
	if key.Rsa == nil && key.Ec == nil {
		panic("invalid key; neither RSA nor ECC")
	}

	if key.Rsa != nil && key.Ec != nil {
		panic("invalid key; neither RSA nor ECC")
	}
}

func (key *SignKey) PubBytes() ([]uint8, error) {
	key.AssertValid()

	var pubkey []byte

	if key.Rsa != nil {
		pubkey, _ = asn1.Marshal(key.Rsa.PublicKey)
	} else {
		switch key.Ec.Curve.Params().Name {
		case "P-224":
			fallthrough
		case "P-256":
			pubkey, _ = x509.MarshalPKIXPublicKey(&key.Ec.PublicKey)
		default:
			return nil, util.NewNewtError("Unsupported ECC curve")
		}
	}

	return pubkey, nil
}

func RawKeyHash(pubKeyBytes []byte) []byte {
	sum := sha256.Sum256(pubKeyBytes)
	return sum[:4]
}

func (key *SignKey) SigLen() uint16 {
	key.AssertValid()

	if key.Rsa != nil {
		return 256
	} else {
		switch key.Ec.Curve.Params().Name {
		case "P-224":
			return 68
		case "P-256":
			return 72
		default:
			return 0
		}
	}
}

func ParsePubKePem(keyBytes []byte) (*rsa.PublicKey, error) {
	b, _ := pem.Decode(keyBytes)
	if b == nil {
		return nil, nil
	}

	if b.Type != "PUBLIC KEY" && b.Type != "RSA PUBLIC KEY" {
		return nil, util.NewNewtError("Invalid PEM file")
	}

	pub, err := x509.ParsePKIXPublicKey(b.Bytes)
	if err != nil {
		return nil, util.FmtNewtError(
			"Error parsing pubkey file: %s", err.Error())
	}

	var pubk *rsa.PublicKey
	switch pub.(type) {
	case *rsa.PublicKey:
		pubk = pub.(*rsa.PublicKey)
	default:
		return nil, util.FmtNewtError(
			"Error parsing pubkey file: %s", err.Error())
	}

	return pubk, nil
}

func ParsePrivKeDer(keyBytes []byte) (*rsa.PrivateKey, error) {
	privKey, err := x509.ParsePKCS1PrivateKey(keyBytes)
	if err != nil {
		return nil, util.FmtNewtError(
			"Error parsing private key file: %s", err.Error())
	}

	return privKey, nil
}

func EncryptSecretRsa(pubk *rsa.PublicKey, plainSecret []byte) ([]byte, error) {
	rng := rand.Reader
	cipherSecret, err := rsa.EncryptOAEP(
		sha256.New(), rng, pubk, plainSecret, nil)
	if err != nil {
		return nil, util.FmtNewtError(
			"Error from encryption: %s\n", err.Error())
	}

	return cipherSecret, nil
}

func DecryptSecretRsa(privk *rsa.PrivateKey,
	cipherSecret []byte) ([]byte, error) {

	rng := rand.Reader
	plainSecret, err := rsa.DecryptOAEP(
		sha256.New(), rng, privk, cipherSecret, nil)
	if err != nil {
		return nil, util.FmtNewtError(
			"Error from encryption: %s\n", err.Error())
	}

	return plainSecret, nil
}

func ParseKeBase64(keyBytes []byte) (cipher.Block, error) {
	kek, err := base64.StdEncoding.DecodeString(string(keyBytes))
	if err != nil {
		return nil, util.FmtNewtError(
			"Error decoding kek: %s", err.Error())
	}
	if len(kek) != 16 {
		return nil, util.FmtNewtError(
			"Unexpected key size: %d != 16", len(kek))
	}

	cipher, err := aes.NewCipher(kek)
	if err != nil {
		return nil, util.FmtNewtError(
			"Error creating keywrap cipher: %s", err.Error())
	}

	return cipher, nil
}

func EncryptSecretAes(c cipher.Block, plainSecret []byte) ([]byte, error) {
	cipherSecret, err := keywrap.Wrap(c, plainSecret)
	if err != nil {
		return nil, util.FmtNewtError("Error key-wrapping: %s", err.Error())
	}

	return cipherSecret, nil
}
