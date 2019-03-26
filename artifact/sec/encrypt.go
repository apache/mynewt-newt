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
