package sec

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"io"

	"mynewt.apache.org/newt/util"
)

func EncryptAES(plain []byte, secret []byte) ([]byte, error) {
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, util.NewNewtError("Failed to create block cipher")
	}
	nonce := make([]byte, 16)
	stream := cipher.NewCTR(block, nonce)

	dataBuf := make([]byte, 16)
	encBuf := make([]byte, 16)
	r := bytes.NewReader(plain)
	w := bytes.Buffer{}
	for {
		cnt, err := r.Read(dataBuf)
		if err != nil && err != io.EOF {
			return nil, util.FmtNewtError(
				"Failed to read from plaintext: %s", err.Error())
		}
		if cnt == 0 {
			break
		}

		stream.XORKeyStream(encBuf, dataBuf[0:cnt])
		if _, err = w.Write(encBuf[0:cnt]); err != nil {
			return nil, util.FmtNewtError(
				"Failed to write ciphertext: %s", err.Error())
		}
	}

	return w.Bytes(), nil
}
