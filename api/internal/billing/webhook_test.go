package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

func TestVerifyWebhook(t *testing.T) {
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	body := []byte(`{"type":"subscription.active","data":{"id":"sub_1"}}`)
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	id := "wh_123"

	t.Run("standard derivation", func(t *testing.T) {
		if err := VerifyWebhook(secret, id, ts, SignWebhook(secret, id, ts, body), body, now); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("legacy derivation, whole string as key", func(t *testing.T) {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(id + "." + ts + "."))
		mac.Write(body)
		sig := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
		if err := VerifyWebhook(secret, id, ts, sig, body, now); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("several signatures, one good", func(t *testing.T) {
		sig := "v1,AAAA v1," + SignWebhook(secret, id, ts, body)[3:] + " v2,BBBB"
		if err := VerifyWebhook(secret, id, ts, sig, body, now); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		other := "whsec_" + base64.StdEncoding.EncodeToString([]byte("ffffffffffffffffffffffffffffffff"))
		if err := VerifyWebhook(secret, id, ts, SignWebhook(other, id, ts, body), body, now); err == nil {
			t.Fatal("verified with the wrong secret")
		}
	})

	t.Run("body changed", func(t *testing.T) {
		if err := VerifyWebhook(secret, id, ts, SignWebhook(secret, id, ts, body), []byte(`{"type":"x"}`), now); err == nil {
			t.Fatal("verified a changed body")
		}
	})

	t.Run("old timestamp", func(t *testing.T) {
		old := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
		if err := VerifyWebhook(secret, id, old, SignWebhook(secret, id, old, body), body, now); err == nil {
			t.Fatal("verified a stale delivery")
		}
	})

	t.Run("missing pieces", func(t *testing.T) {
		if err := VerifyWebhook(secret, "", ts, SignWebhook(secret, id, ts, body), body, now); err == nil {
			t.Fatal("verified without an id")
		}
		if err := VerifyWebhook("", id, ts, SignWebhook(secret, id, ts, body), body, now); err == nil {
			t.Fatal("verified without a secret")
		}
		if err := VerifyWebhook(secret, id, "soon", SignWebhook(secret, id, ts, body), body, now); err == nil {
			t.Fatal("verified a bad timestamp")
		}
	})
}
