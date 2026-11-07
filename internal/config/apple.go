package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

func readAppleFile(name string) ([]byte, error) {
	path := strings.TrimSpace(os.Getenv(name))
	if path == "" {
		return nil, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New(name + " cannot be read")
	}
	return content, nil
}

func loadAppleConfiguration(c *Config) error {
	c.AppleBrowserClientID = strings.TrimSpace(os.Getenv("APPLE_BROWSER_CLIENT_ID"))
	c.AppleBrowserRedirectURI = strings.TrimSpace(os.Getenv("APPLE_BROWSER_REDIRECT_URI"))
	c.AppleBrowserAuthEnabled = envBool("APPLE_BROWSER_AUTH_ENABLED", false)
	c.AppleIdentityClientID = strings.TrimSpace(os.Getenv("APPLE_IDENTITY_CLIENT_ID"))
	c.AppleIdentityTeamID = strings.TrimSpace(os.Getenv("APPLE_IDENTITY_TEAM_ID"))
	c.AppleIdentityKeyID = strings.TrimSpace(os.Getenv("APPLE_IDENTITY_KEY_ID"))
	c.AppleIdentityRedirectURI = strings.TrimSpace(os.Getenv("APPLE_IDENTITY_REDIRECT_URI"))
	var err error
	c.AppleIdentityPrivateKeyPEM, err = readAppleFile("APPLE_IDENTITY_PRIVATE_KEY_PATH")
	if err != nil {
		return err
	}
	if c.AppleIdentityClientID != "" || c.AppleIdentityTeamID != "" || c.AppleIdentityKeyID != "" || len(c.AppleIdentityPrivateKeyPEM) > 0 {
		if c.AppleIdentityClientID == "" || c.AppleIdentityTeamID == "" || c.AppleIdentityKeyID == "" || len(c.AppleIdentityPrivateKeyPEM) == 0 || len(c.OutboxEncryptionKey) != 32 {
			return errors.New("Apple identity requires client/team/key IDs, private key and a separate outbox encryption key")
		}
	}
	if c.AppleBrowserClientID != "" || c.AppleBrowserRedirectURI != "" || c.AppleBrowserAuthEnabled {
		if c.AppleIdentityClientID == "" || c.AppleBrowserClientID == "" || c.AppleBrowserClientID == c.AppleIdentityClientID {
			return errors.New("Apple browser auth requires a distinct Services ID and the native Apple configuration")
		}
		callback, err := url.Parse(c.AppleBrowserRedirectURI)
		if err != nil || callback.Scheme != "https" || callback.Hostname() == "" || callback.User != nil ||
			callback.RawQuery != "" || callback.ForceQuery || callback.Fragment != "" ||
			(callback.EscapedPath() != "/v1/auth/apple/browser/callback" && callback.EscapedPath() != "/api/v1/auth/apple/browser/callback") {
			return errors.New("Apple browser auth requires an HTTPS callback to the Apple browser endpoint")
		}
		appURL, err := url.Parse(c.PublicAppURL)
		if err != nil || appURL.Scheme != "https" || appURL.Hostname() == "" || appURL.User != nil {
			return errors.New("Apple browser auth requires an HTTPS public app URL")
		}
	}
	return nil
}
