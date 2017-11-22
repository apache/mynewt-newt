package image_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"mynewt.apache.org/newt/newt/image"
)

func TestRSA(t *testing.T) {
	signatureTest(t, rsaPkcs1Private)
}

func TestPlainRSAPKCS8(t *testing.T) {
	signatureTest(t, rsaPkcs8Private)
}

func TestEcdsa(t *testing.T) {
	signatureTest(t, ecdsaPrivate)
}

func TestPlainEcdsaPkcs8(t *testing.T) {
	signatureTest(t, ecdsaPkcs8Private)
}

func signatureTest(t *testing.T, privateKey []byte) {
	tmpdir, err := ioutil.TempDir("", "newttest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Create a source image.  Format doesn't really matter that
	// much, since the header will be placed on it by the image
	// tool.

	simpleName := path.Join(tmpdir, "simple.bin")
	hashedName := path.Join(tmpdir, "simple-hashed.bin")
	signedName := path.Join(tmpdir, "simple-signed.bin")
	keyName := path.Join(tmpdir, "private.pem")

	tmp := make([]byte, 256)
	for i := 0; i < len(tmp); i++ {
		tmp[i] = byte(i & 0xFF)
	}
	err = ioutil.WriteFile(simpleName, tmp, 0644)
	if err != nil {
		t.Fatal(err)
	}

	img, err := image.NewImage(simpleName, hashedName)
	if err != nil {
		t.Fatal(err)
	}

	img.SetVersion("1.5")
	if err != nil {
		t.Fatal(err)
	}

	img.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Now try with a signature.
	err = ioutil.WriteFile(keyName, privateKey, 0644)
	if err != nil {
		t.Fatal(err)
	}

	img, err = image.NewImage(simpleName, signedName)
	if err != nil {
		t.Fatal(err)
	}

	err = img.SetSigningKey(keyName, 0)
	if err != nil {
		t.Fatal(err)
	}

	err = img.SetVersion("1.6")
	if err != nil {
		t.Fatal(err)
	}

	err = img.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
}

// An RSA private key in the old PKCS1 format.
var rsaPkcs1Private = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA6q2Q/VoFf6U5xm35ynls+HDbHKwfIbBr27PtFJxlS9YT0xKJ
bcZScPTVizTlft0wfp2TctX/vGd/Y/X3qo5ckRmz+lKUeHm46i4k6rtOBbhBz2id
hwrO7/ylzwaf8lxn2dj/9ikoYQKFtBb/cKu8wyuvW3gs/ou51AVEF8aKTrl5Expy
PrhSlh97er2zUmm8NAoo259I5yHK1SvR9kCw2gNXSDQLpFlK2WikdmEbIu0N+cvN
WM4ONAhffkasznrEOoLPSI66RDrzYhi/Ks9t+N2buEOXao19fDRcSHgZLKT8e6W6
uK7WxRiEzNbajzgDddbZFqWlcpE7sqPNHFBijwIDAQABAoIBAQDdXx7fLpTzROvM
F5/C9GnrraGzWVYAlIgZ9o8Umzceo3GN8PV8fND1xq7Novc9he8h8QjPEbksg0Dz
DWo0FBiTs3hIELAHOWNKXH7sggVmddp2iUvXwEVWsq/CK5CjsbExGXbSQR7a6+Mt
72fEY+wq+0Fuel2PPETuEI2cE+gRuyspIcO7asmMvLRkxLi2EXU0s4JlqV9UfxKQ
aqn0PHlRXa5SIzys3mVhXuoe45T50+VKX0DIfu/RuV8njNkkMx74DeEVvf5W4MJW
vHrRBHoK6KoMrqiwafyPLW/Rh6fMYAdPrffMVuuThtG7Hp83VBVX1HxFhI4Jrf3S
Hf63hmSZAoGBAO2R/vYBl57qgWEKiMQaU8rzctRbP0TtTsqNdISPMnHV1Tn/rNAU
m0N7/6IBDb+IlndXDQwIsW/DTXhF2XJSu7n6GXua8B1LF+zuVWUsFfmE3+eLz7B8
x8G/OkSnOTfRZCYWEoEvzhynn1dlADQ+x49I/XmKqccvAhY71glk6WULAoGBAPzi
IYo9G+ktlNj9/3OciX7aTCiIIMDyPYtYS6wi59gwH9IswaicHYK4w2fDpTWKlzEE
18dKF4puuI5GxnKCwHBiWxGhij063cZKKMqA64X41csK+mumux/PAb2gKbGSzzoF
mSgkKXJ+sZ4ytlgsijEAHV85Sw7j+xy8A0qnCWMNAoGAeCDR7q1hcM8duucrvxWc
90vg7bZyKLVimROsLneGR3+cAWbiiJlS5W3nFpE31XkItLHE/CfNKTl1i/KuAJwL
JwBrMFBpSDa3k2v0rGL9fZ2N5rSQwapnC/ZZTWvNiAcOgB+7Ha4BqAWuke+VidWQ
7Ug4O+Q882Y2xO1ezoNDbX8CgYBq228KyAm8PXuRObsw8iuTg9D8q5ETlwj0kcng
IhvP2X4IxMrMYbOCompHtX9hIYADwaUgXCmYYHLyA+wlRSTmGFmdGKKefvppqLqV
32YmhWBp3Oi2hoy5wzJcG4qis4OHZAg00xsEe464Z3tvxNpcHE1NCJuz3hglKzlE
2VJ5HQKBgQDRisWDbdnOEp7LTXp3Aa33PF1Rx/pkFk4Wb+2Hk977O1OxsAin2cKM
S5HCltHvON2sCmSQUIxMXXKaNPJiGL3UZJxWZDj38zSg0vO+msmemS1Yjt0xCpbO
pkl0kvKb/NVlsY4w9kquvql+t9e1rUu9Ug28TKEsSjc9SFrcnVPoNA==
-----END RSA PRIVATE KEY-----
`)

// An RSA private key in PKCS8 format, with no encryption.
var rsaPkcs8Private = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvwIBADANBgkqhkiG9w0BAQEFAASCBKkwggSlAgEAAoIBAQC+FjuXqPSPucsQ
adxY4nw+9kTgAdsXRIPxq4Q//wkfjEjYhDczN+/rafi0hApuRh7PN7VMGOsDGGR1
edyertiLt3SfUHAZROIqZ0VAoKGtxgXmnC+s+mMujAv9Ssntbmbi5tNxDcltdWjA
SdBn7tbIMVVofKaMMugyuXCglxebMm8yxtkSgUvE1E6zZERnteDJTPo8dBCiqkvU
hf+vG9s1j9lNDMjrZ+d5CHIFmBxJ/WFa6m49lNBFb1Ba43bKdj6mkK05rZ4VWMXU
evy3Z/UUgU4VPJpoB+GIKy82iOrtjiU7s/6aDkvZ2e+fgxKksN0pzFE9azeA73QS
bamp28E/AgMBAAECggEBAJ78+4UDFOKt1JF66YkSjjcfRkZSZwyUCwP0oF3ik5/m
dvtZws29KJevgAyEMDFBxv0srB/k65QgL84uSgATYB2kKRAjeE86VSyASeUfNXui
GEdlNV8p4hEJo/GMP06uu7FmvU1e6a36uM20L3LuyoiQ8s29DJRQ8/ORNQmstlrg
J32FZSjTF1mElGPSc1koxhWvl1hE7UGE9pxsSfdsvPNhCIWwAOnVnIv49xG8EWaK
CkHhEVVdZW8IvO9GYR5U0BJcgzNmdNkS8HVQBIxZtboGAAuPI32EC7siDomKmCF6
rEcs40f/J/RlK6lrTyKKfqWb4DPtRrOSh9cmjrFFZlECgYEA6mZIANLXJd7CINZ9
fjotI+FxH8BDOZF7l8xTnOk1e3Me1ia7t2GMcIL+frfG/zMBiDFq0IQuUYScRK1v
pAILjJKFiU6yY8vH6FZ3mXqiiag6RPa+q89DaUsO0uXRUjQvhtTd5Yy6r8Eac1ya
y6XC5T5sCJ6HgaF3qlheap+5FkkCgYEAz5qSLShV5oekuj1R0fs+h/Yn7VW9Q0sj
px8jOD4MWc8gPZ9gZe0UPTvofOLrM3eAetP4egSif99AE9iD8EbiBzAt16OX7EN8
d7xNiIN922Ep3pubcD6f1vglaI7Thrca/p52g6kWPip6+PWFd1acU6u31Uj0Xvgz
VFiafstF+0cCgYEAw2sOcJFXCZ2Tnyjzav85jwZu95ek9CPUNJQGyXSsQAWUGdok
+hf7q/mqDx9Maoqtpkv8z2bD7vZuCdvGjaee1U16wyS3GPhV69/ayjwxsi5slf5Y
rIiZnPkUnMM5Jh2X2gMyFCSlp82ILdFwxIOn3tOR4gW411w0lfIilSYgevECgYA3
JAgVZHREcdzH9seHrWLze+co+6/0cr26guO46YogRIp8s5tIF0tb5FCg8yijl+cR
OMHzrs12h1aertCEfl9Ep4BVmUcd4uLpbqNtUfeY0FrtnIkRrCCKWYieF+mJC5No
86/o0n1s752QCK51fxSwiJigVutJWkVP7uTCLr2cuwKBgQCJPWMcWmSuRlLOVWnO
jPFoa02Bb83n8GrRpQkpkZZofHextwfo2dd1sZF72zghRsbdC6e0Zj1GrekJOYXO
8AXmCpyKlXJU7iH5tPGSo68uFN05R6mINbTNmEIQBNTKv8UoKT+nHcTycFrVtarX
A8EPW2xB86m+Bjq/GNyRgfbPMg==
-----END PRIVATE KEY-----
`)

// An ECDSA key in the X.509 internal private key format.
var ecdsaPrivate = []byte(`-----BEGIN EC PRIVATE KEY-----
MGgCAQEEHF64kDx3pZyVvezbqYMIxlLbtuPQmI85k4GRy1mgBwYFK4EEACGhPAM6
AASRtolOCTLQYkDefkIF02tUXR92MKHrbtH4WK/8bfTSFVkaygTPdJbpNthK2wae
oX9ZeFHS1pcOfQ==
-----END EC PRIVATE KEY-----
`)

// An ECDSA key in PKCS#8 format, no encryption.
var ecdsaPkcs8Private = []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgHKeDq4UU6M+c+pMm
j0AQZlBs7f4r67668eDCUB8aDR2hRANCAATyZPzsx+xn9JtlxdspevTrYisiMTjl
YuBJCrV1FZj2HkplEgO+ZIMuD7eRvyTEBS2bw6F1aCeKOMUmYVImAbpc
-----END PRIVATE KEY-----
`)
