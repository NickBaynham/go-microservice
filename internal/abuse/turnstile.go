package abuse

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VerifyTurnstile checks a Turnstile widget token with Cloudflare. secret must be non-empty.
// remoteIP is optional (Cloudflare recommends passing it when available).
func VerifyTurnstile(ctx context.Context, secret, responseToken, remoteIP string) (bool, error) {
	secret = strings.TrimSpace(secret)
	responseToken = strings.TrimSpace(responseToken)
	if secret == "" {
		return false, errors.New("turnstile secret not configured")
	}
	if responseToken == "" {
		return false, errors.New("missing turnstile token")
	}

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", responseToken)
	if ip := strings.TrimSpace(remoteIP); ip != "" {
		form.Set("remoteip", ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, err
	}

	var out struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false, err
	}
	return out.Success, nil
}
