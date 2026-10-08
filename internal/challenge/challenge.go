// Package challenge verifies Turnstile and issues shared, one-use receipts.
package challenge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("captcha unavailable")
var ErrRequired = errors.New("captcha required")
var ErrInvalid = errors.New("captcha invalid")

const Lifetime = 120 * time.Second

type Store interface {
	PutProof(context.Context, string, string, time.Duration) (bool, error)
	ConsumeProof(context.Context, string, string) (bool, error)
}
type Manager struct {
	Enabled         bool
	SiteKey, Secret string
	Hostnames       []string
	Store           Store
	Client          *http.Client
	Endpoint        string
}
type receiptKey struct{}

func WithReceipt(ctx context.Context, receipt string) context.Context {
	return context.WithValue(ctx, receiptKey{}, receipt)
}
func validFlow(flow string) bool { return flow == "signup" || flow == "identity" }
func ValidState(state string) bool {
	decoded, e := base64.RawURLEncoding.DecodeString(state)
	return e == nil && len(decoded) >= 24 && len(decoded) <= 64
}
func (m *Manager) Begin(ctx context.Context, flow, state string) error {
	if m == nil || !m.Enabled || m.Store == nil {
		return ErrUnavailable
	}
	if !validFlow(flow) || !ValidState(state) {
		return ErrInvalid
	}
	ok, e := m.Store.PutProof(ctx, "captcha-state:"+state, flow, Lifetime)
	if e != nil {
		return ErrUnavailable
	}
	if !ok {
		return ErrInvalid
	}
	return nil
}
func (m *Manager) Verify(ctx context.Context, token, flow, state, ip string) (string, error) {
	if m == nil || !m.Enabled || m.Store == nil {
		return "", ErrUnavailable
	}
	if !validFlow(flow) || !ValidState(state) || len(token) < 1 || len(token) > 2048 {
		return "", ErrInvalid
	}
	ok, e := m.Store.ConsumeProof(ctx, "captcha-state:"+state, flow)
	if e != nil {
		return "", ErrUnavailable
	}
	if !ok {
		return "", ErrInvalid
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	endpoint := m.Endpoint
	if endpoint == "" {
		endpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	}
	values := url.Values{"secret": {m.Secret}, "response": {token}, "remoteip": {ip}}
	req, e := http.NewRequestWithContext(bounded, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if e != nil {
		return "", ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	response, e := client.Do(req)
	if e != nil {
		return "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", ErrUnavailable
	}
	var result struct {
		Success     bool      `json:"success"`
		Hostname    string    `json:"hostname"`
		Action      string    `json:"action"`
		CData       string    `json:"cdata"`
		ChallengeTS time.Time `json:"challenge_ts"`
	}
	if e = json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&result); e != nil {
		return "", ErrUnavailable
	}
	hostOK := false
	for _, host := range m.Hostnames {
		if host == result.Hostname {
			hostOK = true
		}
	}
	if !result.Success || !hostOK || result.Action != flow || result.CData != state || result.ChallengeTS.IsZero() || time.Since(result.ChallengeTS) > 5*time.Minute || result.ChallengeTS.After(time.Now().Add(time.Minute)) {
		return "", ErrInvalid
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		return "", ErrUnavailable
	}
	receipt := base64.RawURLEncoding.EncodeToString(raw)
	ok, e = m.Store.PutProof(ctx, "captcha-receipt:"+receipt, flow, Lifetime)
	if e != nil || !ok {
		return "", ErrUnavailable
	}
	return receipt, nil
}
func (m *Manager) Check(ctx context.Context, flow string) error {
	if m == nil || !m.Enabled {
		return nil
	}
	if m.Store == nil {
		return ErrUnavailable
	}
	receipt, _ := ctx.Value(receiptKey{}).(string)
	if receipt == "" {
		return ErrRequired
	}
	if len(receipt) != 43 {
		return ErrInvalid
	}
	ok, e := m.Store.ConsumeProof(ctx, "captcha-receipt:"+receipt, flow)
	if e != nil {
		return ErrUnavailable
	}
	if !ok {
		return ErrInvalid
	}
	return nil
}
