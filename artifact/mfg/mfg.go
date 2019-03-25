package mfg

import (
	"crypto/sha256"

	"mynewt.apache.org/newt/util"
)

const MFG_BIN_IMG_FILENAME = "mfgimg.bin"
const MFG_HEX_IMG_FILENAME = "mfgimg.hex"
const MANIFEST_FILENAME = "manifest.json"

type Mfg struct {
	Bin  []byte
	Meta *Meta

	// Unused if Meta==nil.
	MetaOff int
}

func Parse(data []byte, metaEndOff int, eraseVal byte) (Mfg, error) {
	m := Mfg{
		Bin: data,
	}

	if metaEndOff >= 0 {
		meta, _, err := ParseMeta(data[:metaEndOff])
		if err != nil {
			return m, err
		}
		m.Meta = &meta
		m.MetaOff = metaEndOff - int(meta.Footer.Size)
	}

	return m, nil
}

func StripPadding(b []byte, eraseVal byte) []byte {
	var pad int
	for pad = 0; pad < len(b); pad++ {
		off := len(b) - pad - 1
		if b[off] != eraseVal {
			break
		}
	}

	return b[:len(b)-pad]
}

func AddPadding(b []byte, eraseVal byte, padLen int) []byte {
	for i := 0; i < padLen; i++ {
		b = append(b, eraseVal)
	}
	return b
}

// Calculates the SHA256 hash, using the full manufacturing image as input.
// Hash-calculation algorithm is as follows:
// 1. Zero out the 32 bytes that will contain the hash.
// 2. Apply SHA256 to the result.
//
// This function assumes that the 32 bytes of hash data have already been
// zeroed.
func CalcHash(bin []byte) []byte {
	hash := sha256.Sum256(bin)
	return hash[:]
}

func (m *Mfg) RecalcHash(eraseVal byte) error {
	if m.Meta == nil || m.Meta.Hash() == nil {
		return nil
	}

	// First, write with zeroed hash.
	m.Meta.ClearHash()
	bin, err := m.Bytes(eraseVal)
	if err != nil {
		return err
	}

	// Calculate hash and fill TLV.
	tlv := m.Meta.FindFirstTlv(META_TLV_TYPE_HASH)
	if tlv != nil {
		hashData := CalcHash(bin)
		copy(tlv.Data, hashData)

		hashOff := m.MetaOff + m.Meta.HashOffset()
		if hashOff+META_HASH_SZ > len(bin) {
			return util.FmtNewtError(
				"unexpected error: hash extends beyond end " +
					"of manufacturing image")
		}
	}

	return nil
}

func (m *Mfg) Hash() ([]byte, error) {
	var hashBytes []byte
	if m.Meta != nil {
		hashBytes = m.Meta.Hash()
	}
	if hashBytes == nil {
		// No hash TLV; calculate hash manually.
		bin, err := m.Bytes(0xff)
		if err != nil {
			return nil, err
		}
		hashBytes = CalcHash(bin)
	}

	return hashBytes, nil
}

func (m *Mfg) Bytes(eraseVal byte) ([]byte, error) {
	binCopy := make([]byte, len(m.Bin))
	copy(binCopy, m.Bin)

	metaBytes, err := m.Meta.Bytes()
	if err != nil {
		return nil, err
	}

	padLen := m.MetaOff + len(metaBytes) - len(binCopy)
	if padLen > 0 {
		binCopy = AddPadding(binCopy, eraseVal, padLen)
	}

	copy(binCopy[m.MetaOff:m.MetaOff+len(metaBytes)], metaBytes)

	return binCopy, nil
}
