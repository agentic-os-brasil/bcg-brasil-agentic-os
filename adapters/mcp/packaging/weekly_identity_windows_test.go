//go:build windows

package packaging

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type weeklyTrustFixture struct {
	actions                        []uint32
	verifyErr, closeErr, signerErr error
	verified                       uint32
	state                          windows.Handle
	signerHash                     string
	signerReads                    int
	cancel                         context.CancelFunc
}

func (f *weeklyTrustFixture) verify(data *windows.WinTrustData) error {
	f.actions = append(f.actions, data.StateAction)
	if data.StateAction == windows.WTD_STATEACTION_CLOSE {
		return f.closeErr
	}
	data.StateData = f.state
	data.SignatureSettings.VerifiedSigIndex = f.verified
	if f.cancel != nil {
		f.cancel()
	}
	return f.verifyErr
}
func (f *weeklyTrustFixture) signer(state windows.Handle) (string, error) {
	f.signerReads++
	if state != f.state {
		return "", errors.New("wrong trust state")
	}
	return f.signerHash, f.signerErr
}
func weeklyTrustFixtureData() *windows.WinTrustData {
	return &windows.WinTrustData{StateAction: windows.WTD_STATEACTION_VERIFY, SignatureSettings: &windows.WinTrustSignatureSettings{Index: 0, Flags: 1, VerifiedSigIndex: ^uint32(0)}}
}

func TestWeeklyWindowsTrustClosesEveryVerifyPath(t *testing.T) {
	cases := []struct {
		name   string
		change func(*weeklyTrustFixture)
	}{
		{"same state success", func(f *weeklyTrustFixture) {}},
		{"positive nonzero", func(f *weeklyTrustFixture) { f.verifyErr = syscall.Errno(1) }},
		{"warning nonzero", func(f *weeklyTrustFixture) { f.verifyErr = syscall.Errno(0x800b0109) }},
		{"wrong primary index", func(f *weeklyTrustFixture) { f.verified = 1 }},
		{"no trust state", func(f *weeklyTrustFixture) { f.state = 0 }},
		{"invalid trust state", func(f *weeklyTrustFixture) { f.state = windows.InvalidHandle }},
		{"different signer", func(f *weeklyTrustFixture) { f.signerHash = "different" }},
		{"missing helper or malformed metadata", func(f *weeklyTrustFixture) { f.signerErr = errWeeklyWindowsIdentity }},
		{"close error", func(f *weeklyTrustFixture) { f.closeErr = syscall.Errno(1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &weeklyTrustFixture{state: 37, signerHash: "expected"}
			tc.change(f)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			err := weeklyVerifyTrust(ctx, f, weeklyTrustFixtureData(), "expected")
			if (err == nil) != (tc.name == "same state success") {
				t.Fatalf("unexpected verification result: %v", err)
			}
			if len(f.actions) != 2 || f.actions[0] != windows.WTD_STATEACTION_VERIFY || f.actions[1] != windows.WTD_STATEACTION_CLOSE {
				t.Fatalf("trust ownership escaped: %v", f.actions)
			}
			if f.verifyErr != nil && f.signerReads != 0 {
				t.Fatal("failed verification inspected signer")
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	f := &weeklyTrustFixture{state: 37, signerHash: "expected", cancel: cancel}
	if weeklyVerifyTrust(ctx, f, weeklyTrustFixtureData(), "expected") == nil || len(f.actions) != 2 || f.signerReads != 0 {
		t.Fatal("synchronous deadline/cancel returned late success or leaked state")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
	cancel()
	f = &weeklyTrustFixture{}
	if weeklyVerifyTrust(ctx, f, weeklyTrustFixtureData(), "expected") == nil || len(f.actions) != 0 {
		t.Fatal("already cancelled inspection called native verification")
	}
}

func TestWeeklyWindowsTrustRequiresBoundedDeadline(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), nil} {
		f := &weeklyTrustFixture{}
		if weeklyVerifyTrust(ctx, f, weeklyTrustFixtureData(), "expected") == nil || len(f.actions) != 0 {
			t.Fatal("unbounded trust inspection accepted")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 121*time.Second)
	defer cancel()
	f := &weeklyTrustFixture{}
	if weeklyVerifyTrust(ctx, f, weeklyTrustFixtureData(), "expected") == nil || len(f.actions) != 0 {
		t.Fatal("overlong trust inspection accepted")
	}
}

func weeklySignerFixture() (*weeklyProviderSigner, []weeklyProviderCertificate, []byte) {
	// A structurally valid, deliberately unsigned synthetic certificate; no private key.
	algorithm := pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 3, 101, 112}}
	name := asn1.RawValue{Tag: 16, IsCompound: true}
	tbs, _ := asn1.Marshal(struct {
		Version   int `asn1:"explicit,tag:0"`
		Serial    *big.Int
		Signature pkix.AlgorithmIdentifier
		Issuer    asn1.RawValue
		Validity  struct{ Before, After time.Time }
		Subject   asn1.RawValue
		PublicKey struct {
			Algorithm pkix.AlgorithmIdentifier
			Key       asn1.BitString
		}
	}{2, big.NewInt(1), algorithm, name, struct{ Before, After time.Time }{time.Unix(0, 0).UTC(), time.Unix(2000000000, 0).UTC()}, name, struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}{algorithm, asn1.BitString{Bytes: make([]byte, 32), BitLength: 256}}})
	der, _ := asn1.Marshal(struct {
		TBS       asn1.RawValue
		Algorithm pkix.AlgorithmIdentifier
		Signature asn1.BitString
	}{asn1.RawValue{FullBytes: tbs}, algorithm, asn1.BitString{Bytes: make([]byte, 64), BitLength: 512}})
	cert := &windows.CertContext{EncodingType: windows.X509_ASN_ENCODING, EncodedCert: &der[0], Length: uint32(len(der))}
	chain := []weeklyProviderCertificate{{Size: uint32(unsafe.Sizeof(weeklyProviderCertificate{})), Certificate: cert}}
	signer := &weeklyProviderSigner{Size: uint32(unsafe.Sizeof(weeklyProviderSigner{})), ChainCount: 1, Chain: &chain[0], Signer: unsafe.Pointer(&chain[0])}
	return signer, chain, der
}
func TestWeeklyWindowsSelectedPayloadSignerBounds(t *testing.T) {
	t.Run("native signer snapshot", func(t *testing.T) {
		signer, chain, der := weeklySignerFixture()
		var pin runtime.Pinner
		pin.Pin(signer)
		pin.Pin(&chain[0])
		pin.Pin(chain[0].Certificate)
		pin.Pin(&der[0])
		defer pin.Unpin()
		snapshot, err := weeklyReadProviderSigner(uintptr(unsafe.Pointer(signer)))
		if err != nil || snapshot == signer || *snapshot != *signer {
			t.Fatalf("same-process native signer snapshot changed ABI fields: %v", err)
		}
		expected := sha256.Sum256(der)
		actual, err := weeklyProviderSignerSHA256(snapshot, snapshot.Chain)
		if err != nil || actual != hex.EncodeToString(expected[:]) {
			t.Fatalf("snapshot lost original selected certificate: %v", err)
		}
		for _, address := range []uintptr{0, 1} {
			if snapshot, err := weeklyReadProviderSigner(address); err == nil || snapshot != nil {
				t.Fatal("unreadable native signer address accepted")
			}
		}
	})
	signer, chain, der := weeklySignerFixture()
	expected := sha256.Sum256(der)
	actual, err := weeklyProviderSignerSHA256(signer, &chain[0])
	if err != nil || actual != hex.EncodeToString(expected[:]) {
		t.Fatalf("selected same-state certificate rejected: %v", err)
	}
	cases := []struct {
		name   string
		change func(*weeklyProviderSigner)
	}{
		{"unsupported signer ABI", func(s *weeklyProviderSigner) { s.Size-- }},
		{"signer error", func(s *weeklyProviderSigner) { s.Error = 1 }},
		{"timestamp substituted", func(s *weeklyProviderSigner) { s.SignerType = 0x10 }},
		{"no signer metadata", func(s *weeklyProviderSigner) { s.Signer = nil }},
		{"empty chain", func(s *weeklyProviderSigner) { s.ChainCount = 0 }},
		{"oversized chain", func(s *weeklyProviderSigner) { s.ChainCount = 17 }},
		{"nil chain", func(s *weeklyProviderSigner) { s.Chain = nil }},
		{"unsupported cert ABI", func(s *weeklyProviderSigner) { s.Chain.Size-- }},
		{"certificate error", func(s *weeklyProviderSigner) { s.Chain.Error = 1 }},
		{"CTL error", func(s *weeklyProviderSigner) { s.Chain.CTLError = 1 }},
		{"cyclic chain", func(s *weeklyProviderSigner) { s.Chain.Cyclic = 1 }},
		{"test certificate", func(s *weeklyProviderSigner) { s.Chain.TestCertificate = 1 }},
		{"missing certificate", func(s *weeklyProviderSigner) { s.Chain.Certificate = nil }},
		{"missing DER", func(s *weeklyProviderSigner) { s.Chain.Certificate.EncodedCert = nil }},
		{"empty DER", func(s *weeklyProviderSigner) { s.Chain.Certificate.Length = 0 }},
		{"malformed DER", func(s *weeklyProviderSigner) { *s.Chain.Certificate.EncodedCert = 0 }},
		{"oversized DER", func(s *weeklyProviderSigner) { s.Chain.Certificate.Length = 65537 }},
		{"unknown encoding", func(s *weeklyProviderSigner) { s.Chain.Certificate.EncodingType = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := weeklySignerFixture()
			tc.change(s)
			if _, err := weeklyProviderSignerSHA256(s, &c[0]); err == nil {
				t.Fatal("malformed signer metadata accepted")
			}
		})
	}
	unrelated := chain[0]
	if _, err := weeklyProviderSignerSHA256(signer, &unrelated); err == nil {
		t.Fatal("separately discovered certificate adopted")
	}
	if unsafe.Sizeof(weeklyProviderSigner{}) != 64 || unsafe.Sizeof(weeklyProviderCertificate{}) != 88 || unsafe.Offsetof(weeklyProviderSigner{}.Error) != 40 || unsafe.Offsetof(weeklyProviderCertificate{}.Error) != 40 || unsafe.Offsetof(weeklyProviderCertificate{}.ChainElement) != 80 {
		t.Fatal("unsupported Windows 64-bit public SDK ABI")
	}
}
func weeklyEmbeddedPEFixture() []byte {
	body := make([]byte, 416)
	copy(body, "MZ")
	binary.LittleEndian.PutUint32(body[60:64], 64)
	h := body[64:]
	copy(h, "PE\x00\x00")
	binary.LittleEndian.PutUint16(h[4:6], 0x8664)
	binary.LittleEndian.PutUint16(h[6:8], 1)
	binary.LittleEndian.PutUint16(h[20:22], 240)
	binary.LittleEndian.PutUint16(h[22:24], 2)
	o := h[24:]
	binary.LittleEndian.PutUint16(o, 0x20b)
	binary.LittleEndian.PutUint32(o[108:112], 16)
	binary.LittleEndian.PutUint32(o[144:148], 400)
	binary.LittleEndian.PutUint32(o[148:152], 16)
	binary.LittleEndian.PutUint32(body[400:404], 16)
	binary.LittleEndian.PutUint16(body[404:406], 0x200)
	binary.LittleEndian.PutUint16(body[406:408], 2)
	return body
}
func TestWeeklyWindowsEmbeddedNativePEOnly(t *testing.T) {
	body := weeklyEmbeddedPEFixture()
	if weeklyCheckEmbeddedPE(bytes.NewReader(body), (256<<20)+1, "amd64") == nil {
		t.Fatal("Windows256MiB ceiling broadened")
	}
	if weeklyCheckEmbeddedPE(bytes.NewReader(body), int64(len(body)), "386") == nil {
		t.Fatal("32-bit signature topology accepted")
	}
	if body := weeklyEmbeddedPEFixture(); weeklyCheckEmbeddedPE(bytes.NewReader(body), int64(len(body)), "amd64") != nil {
		t.Fatal("bounded synthetic primary PE fixture rejected")
	}
	for _, tc := range []struct {
		name   string
		change func([]byte)
	}{
		{"not PE", func(b []byte) { b[0] = 0 }},
		{"unbounded header offset", func(b []byte) { binary.LittleEndian.PutUint32(b[60:64], ^uint32(0)) }},
		{"wrong target", func(b []byte) { binary.LittleEndian.PutUint16(b[68:70], 0xaa64) }},
		{"DLL", func(b []byte) { binary.LittleEndian.PutUint16(b[86:88], 0x2002) }},
		{"CLR host", func(b []byte) { b[64+24+224] = 1 }},
		{"no embedded signature", func(b []byte) { binary.LittleEndian.PutUint32(b[64+24+148:64+24+152], 0) }},
		{"certificate outside file", func(b []byte) { binary.LittleEndian.PutUint32(b[64+24+144:64+24+148], 416) }},
		{"unsupported certificate type", func(b []byte) { binary.LittleEndian.PutUint16(b[406:408], 1) }},
		{"oversized certificate", func(b []byte) { binary.LittleEndian.PutUint32(b[400:404], 17) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := weeklyEmbeddedPEFixture()
			tc.change(b)
			if weeklyCheckEmbeddedPE(bytes.NewReader(b), int64(len(b)), "amd64") == nil {
				t.Fatal("unsupported embedded PE accepted")
			}
		})
	}
}
