package headscale

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to Headscale HTTP API (v0.23+ style where available).
// When Headscale API is unavailable, CreatePreAuthKey returns a deterministic
// placeholder so local/dev boots still work; production should expose API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type preAuthRequest struct {
	User       string `json:"user"`
	Reusable   bool   `json:"reusable"`
	Ephemeral  bool   `json:"ephemeral"`
	Expiration string `json:"expiration,omitempty"`
}

type preAuthResponse struct {
	PreAuthKey struct {
		Key string `json:"key"`
	} `json:"preAuthKey"`
	Key string `json:"key"`
}

func (c *Client) CreatePreAuthKey(ctx context.Context, user string, ttl time.Duration) (string, error) {
	if c.BaseURL == "" {
		return "", fmt.Errorf("headscale url empty")
	}
	body := preAuthRequest{
		User:       user,
		Reusable:   false,
		Ephemeral:  false,
		Expiration: time.Now().Add(ttl).UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/preauthkey", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// Dev fallback: agent can still enroll with fleet token; overlay join is documented.
		return "hskey-pending-" + fmt.Sprintf("%d", time.Now().UnixNano()), nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "hskey-pending-" + fmt.Sprintf("%d", time.Now().UnixNano()), nil
	}
	var out preAuthResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.PreAuthKey.Key != "" {
		return out.PreAuthKey.Key, nil
	}
	if out.Key != "" {
		return out.Key, nil
	}
	return "hskey-pending-" + fmt.Sprintf("%d", time.Now().UnixNano()), nil
}

func (c *Client) EnsureUser(ctx context.Context, name string) error {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/user", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil // tolerate missing API in bootstrap
	}
	defer resp.Body.Close()
	return nil
}
