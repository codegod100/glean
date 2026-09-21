// Independent check on the Nim DPoP spike.
//
// Self-verification in the same library would only prove internal
// consistency. This re-implements the verifier side from Go's standard
// library -- the same stack the current server runs on -- so agreement here
// means the Nim output is genuinely interoperable, not merely self-coherent.
//
//	./spike/nim/dpop_probe | go run ./verify
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
)

type jwk struct {
	Crv string `json:"crv"`
	Kty string `json:"kty"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type spikeOutput struct {
	JWT        string `json:"jwt"`
	JWK        jwk    `json:"jwk"`
	Thumbprint string `json:"thumbprint"`
}

var failures int

func check(name string, ok bool, detail string) {
	label := "PASS"
	if !ok {
		label = "FAIL"
		failures++
	}
	if detail != "" {
		fmt.Printf("  %s  %s  -- %s\n", label, name, detail)
		return
	}
	fmt.Printf("  %s  %s\n", label, name)
}

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(2)
	}
	var out spikeOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		fmt.Fprintln(os.Stderr, "parse spike output:", err)
		os.Exit(2)
	}

	fmt.Println("Go cross-verification of the Nim DPoP proof")

	parts := strings.Split(out.JWT, ".")
	check("JWT has three parts", len(parts) == 3, fmt.Sprintf("%d", len(parts)))
	if len(parts) != 3 {
		os.Exit(1)
	}

	// Header: ATProto requires typ=dpop+jwt and the embedded public key.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	check("header is unpadded base64url", err == nil, errText(err))
	var header struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
		JWK jwk    `json:"jwk"`
	}
	if err == nil {
		err = json.Unmarshal(headerBytes, &header)
	}
	check("header parses", err == nil, errText(err))
	check("typ is dpop+jwt", header.Typ == "dpop+jwt", header.Typ)
	check("alg is ES256", header.Alg == "ES256", header.Alg)
	check("header jwk matches the reported key",
		header.JWK == out.JWK, "")

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	check("payload is unpadded base64url", err == nil, errText(err))
	var payload map[string]any
	if err == nil {
		err = json.Unmarshal(payloadBytes, &payload)
	}
	check("payload parses", err == nil, errText(err))
	for _, claim := range []string{"jti", "htm", "htu", "iat"} {
		_, ok := payload[claim]
		check("payload has "+claim, ok, "")
	}

	// Signature: JOSE requires raw R||S, not DER. A DER signature here would
	// be the single easiest mistake to make and would fail every PDS.
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	check("signature decodes", err == nil, errText(err))
	check("signature is 64 raw bytes (R||S, not DER)", len(sig) == 64,
		fmt.Sprintf("%d bytes", len(sig)))
	if len(sig) != 64 {
		os.Exit(1)
	}
	check("signature is not DER-wrapped", sig[0] != 0x30, fmt.Sprintf("0x%02x", sig[0]))

	pub, err := publicKey(out.JWK)
	check("JWK converts to a P-256 public key", err == nil, errText(err))
	if err != nil {
		os.Exit(1)
	}

	signingInput := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(signingInput))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	check("ECDSA signature verifies", ecdsa.Verify(pub, digest[:], r, s), "")

	// Negative control: a verifier that accepts anything proves nothing.
	tampered := []byte(signingInput)
	tampered[len(tampered)-1] ^= 0x01
	badDigest := sha256.Sum256(tampered)
	check("a tampered payload is rejected",
		!ecdsa.Verify(pub, badDigest[:], r, s), "")

	// RFC 7638 thumbprint, recomputed independently.
	canonical := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s","y":"%s"}`,
		out.JWK.Crv, out.JWK.Kty, out.JWK.X, out.JWK.Y)
	sum := sha256.Sum256([]byte(canonical))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	check("RFC 7638 thumbprint matches", want == out.Thumbprint, out.Thumbprint)

	fmt.Println()
	if failures == 0 {
		fmt.Println("RESULT: the Nim proof is valid and interoperable.")
		return
	}
	fmt.Printf("RESULT: %d check(s) failed.\n", failures)
	os.Exit(1)
}

func publicKey(k jwk) (*ecdsa.PublicKey, error) {
	if k.Kty != "EC" || k.Crv != "P-256" {
		return nil, fmt.Errorf("unexpected key type %s/%s", k.Kty, k.Crv)
	}
	x, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("x: %w", err)
	}
	y, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf("y: %w", err)
	}
	if len(x) != 32 || len(y) != 32 {
		return nil, fmt.Errorf("coordinates must be 32 bytes, got %d/%d", len(x), len(y))
	}
	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}
	if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
		return nil, fmt.Errorf("point is not on P-256")
	}
	return pub, nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
