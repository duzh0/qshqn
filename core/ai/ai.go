package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"qshqn/core/config"
	"qshqn/core/netx"
	"qshqn/core/qsh"
)

var (
	categories = []string{
		"HARM_CATEGORY_HARASSMENT",
		"HARM_CATEGORY_HATE_SPEECH",
		"HARM_CATEGORY_SEXUALLY_EXPLICIT",
		"HARM_CATEGORY_DANGEROUS_CONTENT",
	}
	blockNone []SafetySetting
)

type Response struct {
	Text         string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Time         float64
}

type ResponseStructured[T any] struct {
	Resp      Response
	Structure T
}

type Structure interface {
	Schema() *Schema
}

func blockNoneSettings() []SafetySetting {
	result := make([]SafetySetting, len(categories))
	for i, cat := range categories {
		result[i] = SafetySetting{
			Category:  cat,
			Threshold: "BLOCK_NONE",
		}
	}
	return result
}

func GenerateText(ctx context.Context, opts *GenerateTextOptions) (r Response, err error) {
	APIkey := CurrentKey()
	if APIkey == "" {
		return r, fmt.Errorf("keys manager current key is empty")
	}
	opts.fillDefaults()
	var contents []Content
	if len(opts.History) > 0 {
		contents = opts.History
	} else {
		contents = make([]Content, 0, 2)
		contents = append(contents, contentFromText(RoleUser, opts.Text))
	}
	payload := ApiRequest{
		Contents:          contents,
		SafetySettings:    blockNone,
		GenerationConfig:  opts.GenConfig,
		SystemInstruction: opts.SystemInstruction,
	}
	url := fmt.Sprintf(config.Services.Gemini.GenerationEndpoint, opts.Model, APIkey)

	maxRetries := 3
	backoff := 5 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, opts.Timeout)

		var response ApiResponse
		startTime := time.Now()
		response, err = netx.PostJSON[ApiResponse](ctxWithTimeout, url, payload, nil)
		responseTime := time.Since(startTime).Seconds()
		cancel()

		if err != nil {
			if e, ok := errors.AsType[*netx.HTTPError](err); ok {
				if (e.StatusCode == 429 || e.StatusCode == 503) && attempt < maxRetries {
					qsh.Warnf("gemini api returned [%d] (attempt %d/%d). retrying in [%v]...", e.StatusCode, attempt+1, maxRetries, backoff)

					select {
					case <-ctx.Done():
						return r, ctx.Err()
					case <-time.After(backoff):
						backoff *= 2
						continue
					}
				}
			}
			return r, fmt.Errorf("gemini api error: %w", err)
		}

		if usage := response.UsageMetadata; usage != nil {
			r.InputTokens = usage.PromptTokenCount
			r.OutputTokens = usage.CandidatesTokenCount
			r.TotalTokens = usage.TotalTokenCount
			r.Time = responseTime
		} else {
			qsh.Warnf("generate content usage metadata is nil? response time: %.3f", responseTime)
		}

		if len(response.Candidates) > 0 {
			cand := response.Candidates[0]
			if len(cand.Content.Parts) > 0 {
				r.Text = cand.Content.Parts[0].Text
				return r, nil
			}

			return r, fmt.Errorf("gemini stopped generation (finish reason: [%s])", cand.FinishReason)
		}

		if response.PromptFeedback != nil && response.PromptFeedback.BlockReason != "" {
			return r, fmt.Errorf("gemini blocked prompt (reason: [%s])", response.PromptFeedback.BlockReason)
		}

		return r, fmt.Errorf("generate content response is empty")
	}

	return r, err
}

func GenerateStructured[T Structure](ctx context.Context, opts *GenerateTextOptions) (r ResponseStructured[T], err error) {
	opts.fillDefaults()

	if opts.GenConfig == nil {
		opts.GenConfig = &GenerationConfig{}
	}
	opts.GenConfig.ResponseMimeType = "application/json"
	opts.GenConfig.ResponseSchema = r.Structure.Schema()

	response, err := GenerateText(ctx, opts)
	r.Resp = response

	if err != nil {
		return r, fmt.Errorf("error generating text: %w", err)
	}

	if err := json.Unmarshal([]byte(response.Text), &r.Structure); err != nil {
		return r, fmt.Errorf("error unmarshalling structured response: %w; raw: %s", err, response.Text)
	}

	return r, nil
}

func RotateKey() string           { return keysMan.ChangeKey() }
func CurrentKey() string          { return keysMan.GetKey() }
func AddKey(key string) error     { return keysMan.Add(key) }
func RemoveKey(idx int) error     { return keysMan.Remove(idx) }
func ListKeys() []string          { return keysMan.List() }
func CurrentKeyIndex() int        { return keysMan.CurrentIndex() }
func ContainsKey(key string) bool { return keysMan.Contains(key) }

// shows first and last 6 chars of api key; returns key on empty string; returns key whole if length <= 12;
//
// example: test_key_123456789 => test_k...56789
func FormatAPIKey(key string) string {
	if key == "" {
		return "none"
	}
	n := len(key)
	if n <= 12 {
		return key
	}
	return key[:6] + "..." + key[n-6:]
}

func Init() error {
	if err := initKeysManager(config.Services.Gemini.APIKeys); err != nil {
		return fmt.Errorf("error initializing keys manager: %w", err)
	}

	blockNone = blockNoneSettings()

	return nil
}
