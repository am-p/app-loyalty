package push

import (
	"clientesFrecuentes/internal/repository"
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	webpush "github.com/SherClockHolmes/webpush-go"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Only browser push services are valid outbound destinations; arbitrary URLs
// and redirects must never become server-side requests.
func ValidateWebSubscription(sub webpush.Subscription) error {
	u, err := url.Parse(sub.Endpoint)
	if err != nil || len(sub.Endpoint) > 4096 || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" || u.Path == "" {
		return errors.New("invalid web push endpoint")
	}
	host := strings.ToLower(u.Hostname())
	allowed := host == "fcm.googleapis.com" || host == "updates.push.services.mozilla.com" || strings.HasSuffix(host, ".push.services.mozilla.com") || host == "web.push.apple.com" || strings.HasSuffix(host, ".push.apple.com") || strings.HasSuffix(host, ".notify.windows.com")
	if !allowed {
		return errors.New("unsupported web push service")
	}
	key, err := base64.RawURLEncoding.DecodeString(sub.Keys.P256dh)
	if err != nil {
		return errors.New("invalid web push public key")
	}
	if _, err = ecdh.P256().NewPublicKey(key); err != nil {
		return errors.New("invalid web push public key")
	}
	auth, err := base64.RawURLEncoding.DecodeString(sub.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return errors.New("invalid web push auth secret")
	}
	return nil
}
func GenerateWebPushKeys() (string, string, error) { return webpush.GenerateVAPIDKeys() }
func NewWebPushHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, errors.New("push service lookup failed")
		}
		for _, candidate := range addresses {
			ip := candidate.IP
			if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				return nil, errors.New("push service address blocked")
			}
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		for _, candidate := range addresses {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, errors.New("push service connection failed")
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("push redirect blocked") }}
}

type WebSender struct {
	HTTP                           webpush.HTTPClient
	PublicKey, PrivateKey, Subject string
}

func (s WebSender) Send(ctx context.Context, job repository.WebPushJob) (bool, error) {
	var subscription webpush.Subscription
	if err := json.Unmarshal([]byte(job.Payload), &subscription); err != nil {
		return true, errors.New("invalid stored subscription")
	}
	if err := ValidateWebSubscription(subscription); err != nil {
		return true, err
	}
	body, _ := json.Marshal(map[string]any{"action": "REFRESH_CARDS", "operation_id": job.OperationID, "card_id": job.CardID, "title": "Tu tarjeta se actualizó", "body": "Tu saldo se actualizó. Abrí Puntazo para verlo."})
	response, err := webpush.SendNotificationWithContext(ctx, body, &subscription, &webpush.Options{HTTPClient: s.HTTP, Subscriber: s.Subject, VAPIDPublicKey: s.PublicKey, VAPIDPrivateKey: s.PrivateKey, TTL: 3600, Urgency: webpush.UrgencyHigh})
	if err != nil {
		return false, errors.New("web push delivery failed")
	}
	defer response.Body.Close()
	if response.StatusCode == 404 || response.StatusCode == 410 {
		return true, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("web push service returned HTTP %d", response.StatusCode)
	}
	return false, nil
}

type WebWorker struct {
	Repo    *repository.Repository
	Logger  *slog.Logger
	Subject string
}

func (w WebWorker) Run(ctx context.Context) {
	if len(w.Repo.OutboxCipherKey) != 32 {
		w.Logger.Warn("web push disabled: outbox encryption key is required")
		return
	}
	client := NewWebPushHTTPClient()
	var sender *WebSender
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	cleanupAt := time.Now()
	keyRetryAt := time.Time{}
	for ctx.Err() == nil {
		if sender == nil && !time.Now().Before(keyRetryAt) {
			keyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			public, private, err := w.Repo.WebPushKeys(keyCtx, GenerateWebPushKeys)
			cancel()
			if err == nil {
				sender = &WebSender{HTTP: client, PublicKey: public, PrivateKey: private, Subject: w.Subject}
			} else {
				w.Logger.Warn("web push keys unavailable")
				keyRetryAt = time.Now().Add(30 * time.Second)
			}
		}
		if sender != nil {
			w.process(ctx, *sender)
		}
		if time.Since(cleanupAt) >= time.Hour {
			cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, _ = w.Repo.Pool.Exec(cleanupCtx, `DELETE FROM web_push_notifications WHERE estado IN ('SENT','FAILED') AND updated_at<now()-interval '30 days'`)
			cancel()
			cleanupAt = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w WebWorker) process(ctx context.Context, sender WebSender) {
	claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	jobs, err := w.Repo.ClaimWebPushJobs(claimCtx)
	cancel()
	if err != nil {
		w.Logger.Error("web push claim failed")
		return
	}
	for _, job := range jobs {
		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if !w.Repo.WebPushJobActive(sendCtx, job) {
			cancel()
			continue
		}
		expired, sendErr := sender.Send(sendCtx, job)
		cancel()
		resultCtx, resultCancel := context.WithTimeout(ctx, 5*time.Second)
		err = w.Repo.FinishWebPushJob(resultCtx, job, sendErr == nil, expired)
		resultCancel()
		if sendErr != nil {
			w.Logger.Warn("web push delivery retry", "job_id", job.ID, "attempt", job.Attempts)
		}
		if err != nil {
			w.Logger.Error("web push result save failed", "job_id", job.ID)
		}
	}
}
