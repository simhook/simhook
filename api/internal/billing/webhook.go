package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrWebhookSignature says a delivery did not verify against the secret.
var ErrWebhookSignature = errors.New("webhook signature does not verify")

// webhookTolerance is how far a delivery's timestamp may be from now. It
// bounds replay of a captured delivery.
const webhookTolerance = 5 * time.Minute

// VerifyWebhook checks a delivery against the endpoint secret as Polar
// hands it out ("whsec_" and a base64 string). Polar signs per Standard
// Webhooks: HMAC-SHA256 over "<id>.<timestamp>.<body>", and the signature
// header carries one or more "v1,<base64>" entries, any of which may
// match. Two key derivations exist: a secret issued before 8 September
// 2026 signs with the UTF-8 bytes of the whole string, a later one with
// the base64 decoding of what follows "whsec_". Both are tried, so a
// secret of either age verifies without configuration saying which.
func VerifyWebhook(secret, id, timestamp, signature string, body []byte, now time.Time) error {
	if secret == "" || id == "" || timestamp == "" || signature == "" {
		return ErrWebhookSignature
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return ErrWebhookSignature
	}
	if skew := now.Sub(time.Unix(ts, 0)); skew > webhookTolerance || skew < -webhookTolerance {
		return ErrWebhookSignature
	}
	msg := make([]byte, 0, len(id)+len(timestamp)+len(body)+2)
	msg = append(msg, id...)
	msg = append(msg, '.')
	msg = append(msg, timestamp...)
	msg = append(msg, '.')
	msg = append(msg, body...)

	keys := [][]byte{[]byte(secret)}
	if raw, ok := strings.CutPrefix(secret, "whsec_"); ok {
		if k, err := base64.StdEncoding.DecodeString(raw); err == nil {
			keys = append(keys, k)
		}
	}
	want := make([][]byte, 0, len(keys))
	for _, k := range keys {
		mac := hmac.New(sha256.New, k)
		mac.Write(msg)
		want = append(want, mac.Sum(nil))
	}
	for _, part := range strings.Fields(signature) {
		version, sig, ok := strings.Cut(part, ",")
		if !ok || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			continue
		}
		for _, w := range want {
			if hmac.Equal(got, w) {
				return nil
			}
		}
	}
	return ErrWebhookSignature
}

// SignWebhook produces the signature header value Polar would send for a
// delivery, under the Standard Webhooks derivation. Tests use it.
func SignWebhook(secret, id, timestamp string, body []byte) string {
	key := []byte(secret)
	if raw, ok := strings.CutPrefix(secret, "whsec_"); ok {
		if k, err := base64.StdEncoding.DecodeString(raw); err == nil {
			key = k
		}
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
