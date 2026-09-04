// Package secrets holds every primitive that touches a credential: random
// token generation, hashing for lookup, password hashing, and symmetric
// encryption for values that must be recoverable.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Token prefixes make a leaked credential recognisable at a glance and let
// scanners flag them.
const (
	PrefixAPIKey  = "sh_"
	PrefixDevice  = "shd_"
	PrefixSession = "shs_"
)

// NewToken returns a random token with the given prefix and its SHA-256
// hash. Only the hash is ever stored.
func NewToken(prefix string) (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("secrets: random: %w", err)
	}
	token = prefix + base64.RawURLEncoding.EncodeToString(raw)
	return token, Hash(token), nil
}

// Hash is the deterministic lookup hash for a token.
func Hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// DisplayPrefix returns the part of a token safe to show in a list, for
// example sh_a1b2c3d4.
func DisplayPrefix(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:12]
}

// NewPairingCode returns a short human-typable code (8 characters from an
// unambiguous alphabet) and its hash.
func NewPairingCode() (code string, hash []byte, err error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("secrets: random: %w", err)
	}
	var sb strings.Builder
	for i, b := range raw {
		if i == 4 {
			sb.WriteByte('-')
		}
		sb.WriteByte(alphabet[int(b)%len(alphabet)])
	}
	code = sb.String()
	return code, Hash(NormalizePairingCode(code)), nil
}

// NormalizePairingCode makes user input comparable: uppercase, no dashes or
// spaces.
func NormalizePairingCode(input string) string {
	s := strings.ToUpper(strings.TrimSpace(input))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// NewOneTimeCode returns a 6-digit numeric code and its hash, for email
// verification and password reset.
func NewOneTimeCode() (code string, hash []byte, err error) {
	raw := make([]byte, 4)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("secrets: random: %w", err)
	}
	n := (uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])) % 1000000
	code = fmt.Sprintf("%06d", n)
	return code, Hash(code), nil
}

// NewWebhookSecret returns a random signing secret for a webhook.
func NewWebhookSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("secrets: random: %w", err)
	}
	return "whsec_" + hex.EncodeToString(raw), nil
}

// ---------------------------------------------------------------------------
// Passwords: Argon2id, encoded in the standard PHC string format.
// ---------------------------------------------------------------------------

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

// HashPassword returns a PHC-formatted Argon2id hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("secrets: random: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the PHC hash.
func VerifyPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---------------------------------------------------------------------------
// Symmetric encryption for values that must be read back (webhook secrets).
// ---------------------------------------------------------------------------

// Box encrypts and decrypts with AES-256-GCM under a fixed key.
type Box struct {
	aead cipher.AEAD
}

// NewBox builds a Box from a 32-byte key.
func NewBox(key []byte) (*Box, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. The nonce is prepended to the ciphertext.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a value produced by Seal.
func (b *Box) Open(ciphertext []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(ciphertext) < n {
		return nil, errors.New("secrets: ciphertext too short")
	}
	return b.aead.Open(nil, ciphertext[:n], ciphertext[n:], nil)
}

// ---------------------------------------------------------------------------
// Webhook signatures.
// ---------------------------------------------------------------------------

// SignWebhook returns the value for the X-Simhook-Signature header:
// t=<unix seconds>,v1=<hex hmac-sha256 over "<t>.<body>">.
func SignWebhook(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", timestamp)
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}

// VerifyWebhook checks a signature header against a body. It is used by
// tests and by the SDK's reference implementation.
func VerifyWebhook(secret, header string, body []byte, now int64, tolerance int64) bool {
	var ts int64
	var sig string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return false
		}
		switch k {
		case "t":
			if _, err := fmt.Sscanf(v, "%d", &ts); err != nil {
				return false
			}
		case "v1":
			sig = v
		}
	}
	if ts == 0 || sig == "" {
		return false
	}
	if d := now - ts; d > tolerance || d < -tolerance {
		return false
	}
	want := SignWebhook(secret, ts, body)
	return hmac.Equal([]byte(want), []byte(fmt.Sprintf("t=%d,v1=%s", ts, sig)))
}
