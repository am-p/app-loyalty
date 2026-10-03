package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"clientesFrecuentes/internal/repository"
	webpush "github.com/SherClockHolmes/webpush-go"
)

type webHTTP func(*http.Request) (*http.Response, error)

func (f webHTTP) Do(r *http.Request) (*http.Response, error) { return f(r) }
func validWebSubscription(t *testing.T) webpush.Subscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return webpush.Subscription{Endpoint: "https://fcm.googleapis.com/fcm/send/test", Keys: webpush.Keys{P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16))}}
}
func TestWebPushValidatesProvidersAndKeys(t *testing.T) {
	sub := validWebSubscription(t)
	for _, endpoint := range []string{"https://fcm.googleapis.com/fcm/send/token", "https://updates.push.services.mozilla.com/wpush/v2/token", "https://web.push.apple.com/token"} {
		sub.Endpoint = endpoint
		if err := ValidateWebSubscription(sub); err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range []string{"http://fcm.googleapis.com/send/token", "https://localhost/token", "https://fcm.googleapis.com.evil.test/token", "https://fcm.googleapis.com:8443/token", "https://name@fcm.googleapis.com/token", "https://fcm.googleapis.com/token#fragment"} {
		sub.Endpoint = endpoint
		if ValidateWebSubscription(sub) == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	sub = validWebSubscription(t)
	sub.Keys.Auth = "invalid"
	if ValidateWebSubscription(sub) == nil {
		t.Fatal("accepted invalid auth")
	}
	sub = validWebSubscription(t)
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	if ValidateWebSubscription(sub) == nil {
		t.Fatal("accepted invalid curve key")
	}
}
func TestWebPushEncryptsPayloadAndHandlesProviderResults(t *testing.T) {
	private, public, err := GenerateWebPushKeys()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(validWebSubscription(t))
	for _, status := range []int{201, 410, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			called := false
			sender := WebSender{PublicKey: public, PrivateKey: private, Subject: "https://testing.puntazo.pro", HTTP: webHTTP(func(req *http.Request) (*http.Response, error) {
				called = true
				body, _ := io.ReadAll(req.Body)
				if req.Header.Get("Content-Encoding") != "aes128gcm" || !strings.HasPrefix(req.Header.Get("Authorization"), "vapid ") || strings.Contains(string(body), "REFRESH_CARDS") {
					t.Fatal("unencrypted or unsigned push")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			expired, err := sender.Send(context.Background(), repository.WebPushJob{Payload: string(payload), OperationID: "11111111-1111-4111-8111-111111111111", CardID: 1})
			if !called || expired != (status == 410) || (err != nil) != (status == 503) {
				t.Fatalf("status %d expired %v err %v", status, expired, err)
			}
		})
	}
}
