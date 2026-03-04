package voicebox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const (
	defaultSTTTimeoutSeconds = 30
	sttTranscribePath        = "/transcribe_audio"
)

// STTProxy sends captured audio to a configurable STT proxy endpoint.
type STTProxy struct {
	endpoint       string
	apiKey         string
	tenantID       string
	timeoutSeconds int
}

func NewSTTProxy(endpoint, apiKey, tenantID string, timeoutSeconds int) *STTProxy {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultSTTTimeoutSeconds
	}
	return &STTProxy{
		endpoint:       endpoint,
		apiKey:         strings.TrimSpace(apiKey),
		tenantID:       strings.TrimSpace(tenantID),
		timeoutSeconds: timeoutSeconds,
	}
}

func (s *STTProxy) Transcribe(ctx context.Context, audio []byte) (string, error) {
	if s == nil || len(audio) == 0 {
		return "", nil
	}

	url := s.endpoint
	if !strings.Contains(url, "/audio/transcriptions") && !strings.HasSuffix(url, sttTranscribePath) {
		url = strings.TrimRight(url, "/") + sttTranscribePath
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", "voice.opus")
	if err != nil {
		return "", fmt.Errorf("stt: create file part: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", fmt.Errorf("stt: write audio bytes: %w", err)
	}
	if s.tenantID != "" {
		if err := w.WriteField("tenant_id", s.tenantID); err != nil {
			return "", fmt.Errorf("stt: write tenant_id: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("stt: finalize multipart: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(s.timeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("stt: build request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("stt: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("stt: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt: upstream status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var parsed struct {
		Transcript string `json:"transcript"`
		Text       string `json:"text"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("stt: decode response: %w", err)
	}
	text := strings.TrimSpace(parsed.Transcript)
	if text == "" {
		text = strings.TrimSpace(parsed.Text)
	}
	return text, nil
}
