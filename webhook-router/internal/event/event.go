package event

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Payload map[string]any

type WebhookEvent struct {
	ID         string      `json:"id"`
	SourceID   string      `json:"source_id"`
	Channel    string      `json:"channel"`
	ReceivedAt time.Time   `json:"received_at"`
	Headers    http.Header `json:"headers"`
	Body       any         `json:"body,omitempty"`
	BodyBase64 string      `json:"body_base64,omitempty"`
}

func BuildBody(raw []byte, contentType string) (any, string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.Contains(strings.ToLower(contentType), "json") || json.Valid(raw) {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, "", err
		}
		return value, "", nil
	}
	if utf8.Valid(raw) {
		return map[string]any{"raw": trimmed}, "", nil
	}
	return nil, base64.StdEncoding.EncodeToString(raw), nil
}

func Marshal(e WebhookEvent) ([]byte, error) {
	return json.Marshal(e)
}

func Unmarshal(raw string) (WebhookEvent, error) {
	var e WebhookEvent
	err := json.Unmarshal([]byte(raw), &e)
	return e, err
}
