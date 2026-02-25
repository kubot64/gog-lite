package googleapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries429      = 3
	maxRetries5xx      = 1
	rateLimitBaseDelay = 1 * time.Second
	serverErrorDelay   = 1 * time.Second
)

// RetryTransport wraps an http.RoundTripper with retry logic for 429 and 5xx.
type RetryTransport struct {
	Base          http.RoundTripper
	MaxRetries429 int
	MaxRetries5xx int
	BaseDelay     time.Duration
}

// NewRetryTransport creates a RetryTransport with sensible defaults.
func NewRetryTransport(base http.RoundTripper) *RetryTransport {
	if base == nil {
		base = http.DefaultTransport
	}

	return &RetryTransport{
		Base:          base,
		MaxRetries429: maxRetries429,
		MaxRetries5xx: maxRetries5xx,
		BaseDelay:     rateLimitBaseDelay,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := ensureReplayableBody(req); err != nil {
		return nil, err
	}

	retries429 := 0
	retries5xx := 0

	for {
		if req.GetBody != nil {
			if req.Body != nil {
				_ = req.Body.Close()
			}

			body, getErr := req.GetBody()
			if getErr != nil {
				return nil, fmt.Errorf("reset request body: %w", getErr)
			}

			req.Body = body
		}

		resp, err := t.Base.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("round trip: %w", err)
		}

		if resp.StatusCode < 400 {
			return resp, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if retries429 >= t.MaxRetries429 {
				return resp, nil
			}

			delay := t.calculateBackoff(retries429, resp)
			drainAndClose(resp.Body)

			if err := t.sleep(req.Context(), delay); err != nil {
				return nil, err
			}

			retries429++

			continue
		}

		if resp.StatusCode >= 500 {
			if retries5xx >= t.MaxRetries5xx {
				return resp, nil
			}

			drainAndClose(resp.Body)

			if err := t.sleep(req.Context(), serverErrorDelay); err != nil {
				return nil, err
			}

			retries5xx++

			continue
		}

		return resp, nil
	}
}

func (t *RetryTransport) calculateBackoff(attempt int, resp *http.Response) time.Duration {
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			if seconds < 0 {
				return 0
			}

			return time.Duration(seconds) * time.Second
		}

		if parsed, err := http.ParseTime(retryAfter); err == nil {
			d := time.Until(parsed)
			if d < 0 {
				return 0
			}

			return d
		}
	}

	if t.BaseDelay <= 0 {
		return 0
	}

	baseDelay := t.BaseDelay * time.Duration(1<<attempt)
	if baseDelay <= 0 {
		return 0
	}

	jitterRange := baseDelay / 2
	if jitterRange <= 0 {
		return baseDelay
	}

	jitter := time.Duration(rand.Int64N(int64(jitterRange))) //nolint:gosec

	return baseDelay + jitter
}

func (t *RetryTransport) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("sleep interrupted: %w", ctx.Err())
	}
}

func newBaseTransport() *http.Transport {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || defaultTransport == nil {
		return &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
	}

	transport := defaultTransport.Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

		return transport
	}

	if transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		transport.TLSClientConfig.MinVersion = tls.VersionTLS12
	}

	return transport
}

// bytesReader is a simple replayable bytes reader.
type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n

	return n, nil
}

func ensureReplayableBody(req *http.Request) error {
	if req == nil || req.Body == nil || req.GetBody != nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}

	_ = req.Body.Close()

	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(newBytesReader(bodyBytes)), nil
	}
	req.Body = io.NopCloser(newBytesReader(bodyBytes))

	return nil
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}
