package crc16

import (
	"testing"
)

type testpair struct {
	values []byte
	kermit uint16
}

var tests = []testpair{
	{[]byte("Hello"), 0x5B9B},
	{[]byte("123456789"), 0x8921},
	{[]byte("jsaldfjasljdfuouuxvasfsfasf"), 0x67BE},
	{[]byte("987654321"), 0x9F34},
	{[]byte("987654321987654321987654321987654321987654321987654321987654321987654321987654321987654321987654321987654321987654321987654321"), 0xAF56},
}

func TestKermit(t *testing.T) {
	for _, pair := range tests {
		v := Kermit(pair.values)
		if v != pair.kermit {
			t.Error(
				"For", pair.values,
				"expected", pair.kermit,
				"got", v,
			)
		}
	}
}
