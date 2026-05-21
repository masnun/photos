package classifier

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const endpoint = "https://api.anthropic.com/v1/messages"

const SystemPrompt = `You classify photographs by genre. Respond with strict JSON only, no markdown fences.

Schema:
{"genres": ["..."], "caption": "...", "collection_hint": "..."}

Available genres (kebab-case slugs). Pick all that apply — multi-label is encouraged. You MAY propose a new slug only if none of these fit:
street, portrait, landscape, architecture, nature, wildlife, macro, abstract, documentary, travel, urban, black-and-white, still-life, event, sports, fashion, food, astrophotography, night, candid, minimalism, cityscape, interior

Rules:
- genres: 1-4 slugs, lowercase, kebab-case.
- caption: under 100 chars, concrete description of the scene. Empty string if unsure.
- collection_hint: optional kebab-case slug suggesting a logical group (e.g. "tokyo-street", "winter-fog"). Empty string if no obvious grouping.
- If the image is monochrome, always include "black-and-white".`

type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

func New(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 90 * time.Second},
	}
}

type Result struct {
	Genres         []string `json:"genres"`
	Caption        string   `json:"caption"`
	CollectionHint string   `json:"collection_hint"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type contentBlock struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *imageSource `json:"source,omitempty"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type requestBody struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []message `json:"messages"`
}

type response struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Classify(ctx context.Context, filename string, jpegData []byte) (*Result, error) {
	body := requestBody{
		Model:     c.model,
		MaxTokens: 400,
		System:    SystemPrompt,
		Messages: []message{{
			Role: "user",
			Content: []contentBlock{
				{Type: "image", Source: &imageSource{
					Type:      "base64",
					MediaType: "image/jpeg",
					Data:      base64.StdEncoding.EncodeToString(jpegData),
				}},
				{Type: "text", Text: "Classify this photo. Filename: " + filename},
			},
		}},
	}

	raw, err := c.callWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}

	var parsed response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w (body: %s)", err, truncate(string(raw), 300))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("api error: %s", parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		return nil, fmt.Errorf("empty content")
	}
	text := stripFences(strings.TrimSpace(parsed.Content[0].Text))

	var r Result
	if err := json.Unmarshal([]byte(text), &r); err != nil {
		return nil, fmt.Errorf("decode model json: %w (text: %s)", err, truncate(text, 300))
	}
	r.Genres = normalizeSlugs(r.Genres)
	r.CollectionHint = normalizeSlug(r.CollectionHint)
	return &r, nil
}

type retryable struct {
	status int
	msg    string
}

func (r *retryable) Error() string { return fmt.Sprintf("status %d: %s", r.status, r.msg) }

func (c *Client) callWithRetry(ctx context.Context, body requestBody) ([]byte, error) {
	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt < 5; attempt++ {
		bb, err := c.call(ctx, body)
		if err == nil {
			return bb, nil
		}
		lastErr = err
		if _, ok := err.(*retryable); !ok {
			return nil, err
		}
		r := err.(*retryable)
		if r.status != 429 && r.status < 500 {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	return nil, fmt.Errorf("max retries: %w", lastErr)
}

func (c *Client) call(ctx context.Context, body requestBody) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &retryable{status: resp.StatusCode, msg: truncate(string(raw), 300)}
	}
	return raw, nil
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i > 0 {
			s = s[i+1:]
		}
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func normalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func normalizeSlugs(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = normalizeSlug(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
