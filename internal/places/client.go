// Package places accesses only the official Places API (New), never merchant supplied links.
package places

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"
)

var ErrUnavailable = errors.New("places unavailable")

type Client struct {
	APIKey  string
	HTTP    *http.Client
	BaseURL string
}

func New(key string) *Client {
	return &Client{APIKey: key, HTTP: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, BaseURL: "https://places.googleapis.com/v1"}
}
func (c *Client) Available() bool { return c != nil && strings.TrimSpace(c.APIKey) != "" }

// OfficialReviewURL also protects client link opening when a provider returns malformed data.
func OfficialReviewURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Host != strings.ToLower(u.Host) || u.Fragment != "" {
		return false
	}
	switch u.Host {
	case "g.page", "maps.app.goo.gl", "maps.google.com", "www.google.com", "search.google.com", "share.google":
		return u.Path != "" && u.Path != "/"
	}
	return false
}

type place struct {
	Attributions []struct {
		Provider    string `json:"provider"`
		ProviderURI string `json:"providerUri"`
	} `json:"attributions"`
	ID          string `json:"id"`
	DisplayName struct {
		Text string `json:"text"`
	} `json:"displayName"`
	Address string `json:"formattedAddress"`
	Links   struct {
		Write string `json:"writeAReviewUri"`
	} `json:"googleMapsLinks"`
}

func link(raw string) *string {
	if !OfficialReviewURL(raw) {
		return nil
	}
	return &raw
}
func (c *Client) request(ctx context.Context, method, path, mask string, body any, out any) error {
	if !c.Available() {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var b []byte
	var e error
	if body != nil {
		b, e = json.Marshal(body)
		if e != nil {
			return ErrUnavailable
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(b))
	if e != nil {
		return ErrUnavailable
	}
	req.Header.Set("X-Goog-Api-Key", c.APIKey)
	req.Header.Set("X-Goog-FieldMask", mask)
	req.Header.Set("Content-Type", "application/json")
	res, e := c.HTTP.Do(req)
	if e != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return ErrUnavailable
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (c *Client) Search(ctx context.Context, q string) ([]model.ReviewPlace, error) {
	var out struct {
		Places []place `json:"places"`
	}
	if e := c.request(ctx, "POST", "/places:searchText", "places.id,places.displayName,places.formattedAddress,places.googleMapsLinks.writeAReviewUri,places.attributions", map[string]any{"textQuery": q, "languageCode": "es", "pageSize": 10}, &out); e != nil {
		return nil, e
	}
	result := []model.ReviewPlace{}
	for _, p := range out.Places {
		attribution := []model.ReviewAttribution{}
		for _, a := range p.Attributions {
			u, e := url.Parse(a.ProviderURI)
			if e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Port() == "" && strings.TrimSpace(a.Provider) != "" {
				attribution = append(attribution, model.ReviewAttribution{Provider: a.Provider, ProviderURI: a.ProviderURI})
			}
		}
		result = append(result, model.ReviewPlace{PlaceID: p.ID, Name: p.DisplayName.Text, Address: p.Address, ReviewURL: link(p.Links.Write), Attributions: attribution})
	}
	return result, nil
}
func (c *Client) Resolve(ctx context.Context, id string) (string, error) {
	var p place
	if e := c.request(ctx, "GET", "/places/"+url.PathEscape(id), "googleMapsLinks.writeAReviewUri", nil, &p); e != nil {
		return "", e
	}
	if !OfficialReviewURL(p.Links.Write) {
		return "", ErrUnavailable
	}
	return p.Links.Write, nil
}
