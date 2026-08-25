package netx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	MIME_JPEG  = "image/jpeg"
	MIME_PNG   = "image/png"
	MIME_JSON  = "application/json"
	MIME_TXT   = "text/plain"
	MIME_BYTES = "bytes"

	DEFAULT_MAX_BODY_SIZE = 2 * 1024 * 1024
)

var client = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	},
}

type HTTPError struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %s: %s", e.Status, string(e.Body))
}

type RequestOptions struct {
	MaxBodySize int64
}

func (o *RequestOptions) setDefaults() {
	if o.MaxBodySize == 0 {
		o.MaxBodySize = DEFAULT_MAX_BODY_SIZE
	}
}

func ensureOpts(o *RequestOptions) *RequestOptions {
	if o == nil {
		o = &RequestOptions{}
	}
	o.setDefaults()
	return o
}

func readLimited(r io.Reader, size int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, size))
}

func do(ctx context.Context, method, url string, bodyData []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if bodyData != nil {
		bodyReader = bytes.NewReader(bodyData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest error: %w", err)
	}

	if bodyData != nil {
		req.Header.Set("Content-Type", MIME_JSON)
	}

	r, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do error: %w", err)
	}

	if r.StatusCode < 200 || r.StatusCode > 299 {
		defer r.Body.Close()
		body, _ := readLimited(r.Body, DEFAULT_MAX_BODY_SIZE)
		return nil, &HTTPError{
			StatusCode: r.StatusCode,
			Status:     r.Status,
			Body:       body,
		}
	}

	return r, nil
}

func Get(ctx context.Context, url string, opts *RequestOptions) ([]byte, error) {
	opts = ensureOpts(opts)

	r, err := do(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	b, err := readLimited(r.Body, opts.MaxBodySize)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}
	return b, nil
}

func DoJSON(ctx context.Context, method, url string, reqBody any, dest any, opts *RequestOptions) error {
	opts = ensureOpts(opts)

	var reqBytes []byte
	var err error
	if reqBody != nil {
		if reqBytes, err = json.Marshal(reqBody); err != nil {
			return fmt.Errorf("error marshalling reqBody: %w", err)
		}
	}

	r, err := do(ctx, method, url, reqBytes)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(io.LimitReader(r.Body, opts.MaxBodySize)).Decode(dest)
}

func GetJSON[T any](ctx context.Context, url string, opts *RequestOptions) (T, error) {
	var dest T
	err := DoJSON(ctx, http.MethodGet, url, nil, &dest, opts)
	return dest, err
}

func PostJSON[T any](ctx context.Context, url string, body any, opts *RequestOptions) (T, error) {
	var dest T
	err := DoJSON(ctx, http.MethodPost, url, body, &dest, opts)
	return dest, err
}
