package googleapi

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// mockTransport is a simple mock RoundTripper that returns pre-configured responses.
type mockTransport struct {
	responses []*http.Response
	callCount int
}

func (m *mockTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	if m.callCount >= len(m.responses) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func mockResp(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

// noDelayTransport wraps mockTransport and returns fast responses for testing.
func noDelayRT(mock *mockTransport) *RetryTransport {
	return &RetryTransport{
		Base:          mock,
		MaxRetries429: 3,
		MaxRetries5xx: 1,
		BaseDelay:     0, // disables exponential backoff sleep
	}
}

func newGETRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	return req
}

func TestRetryTransport_200NoRetry(t *testing.T) {
	mock := &mockTransport{responses: []*http.Response{mockResp(200)}}
	resp, err := noDelayRT(mock).RoundTrip(newGETRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	if mock.callCount != 1 {
		t.Errorf("want 1 call, got %d", mock.callCount)
	}
}

func TestRetryTransport_404NoRetry(t *testing.T) {
	mock := &mockTransport{responses: []*http.Response{mockResp(404)}}
	resp, err := noDelayRT(mock).RoundTrip(newGETRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
	if mock.callCount != 1 {
		t.Errorf("want 1 call (no retry for 4xx), got %d", mock.callCount)
	}
}

func TestRetryTransport_429RetriesToSuccess(t *testing.T) {
	mock := &mockTransport{responses: []*http.Response{
		mockResp(429), mockResp(429), mockResp(200),
	}}
	resp, err := noDelayRT(mock).RoundTrip(newGETRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("want 200 after retries, got %d", resp.StatusCode)
	}
	if mock.callCount != 3 {
		t.Errorf("want 3 calls, got %d", mock.callCount)
	}
}

func TestRetryTransport_429ExceedsMax(t *testing.T) {
	// MaxRetries429=3: retries on attempts 0,1,2 then returns on attempt 3 → 4 total calls
	mock := &mockTransport{responses: []*http.Response{
		mockResp(429), mockResp(429), mockResp(429), mockResp(429),
	}}
	resp, err := noDelayRT(mock).RoundTrip(newGETRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 429 {
		t.Errorf("want 429 after max retries exceeded, got %d", resp.StatusCode)
	}
	if mock.callCount != 4 {
		t.Errorf("want 4 calls (3 retries + 1 final), got %d", mock.callCount)
	}
}

func TestRetryTransport_500NoRetryWhenMaxZero(t *testing.T) {
	mock := &mockTransport{responses: []*http.Response{mockResp(500)}}
	rt := &RetryTransport{Base: mock, MaxRetries429: 3, MaxRetries5xx: 0, BaseDelay: 0}
	resp, err := rt.RoundTrip(newGETRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("want 500, got %d", resp.StatusCode)
	}
	if mock.callCount != 1 {
		t.Errorf("want 1 call, got %d", mock.callCount)
	}
}

func TestRetryTransport_500RetriesOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test: sleeps 1s for 5xx backoff")
	}
	mock := &mockTransport{responses: []*http.Response{
		mockResp(500), mockResp(200),
	}}
	rt := &RetryTransport{Base: mock, MaxRetries429: 3, MaxRetries5xx: 1, BaseDelay: 0}
	resp, err := rt.RoundTrip(newGETRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("want 200 after 5xx retry, got %d", resp.StatusCode)
	}
	if mock.callCount != 2 {
		t.Errorf("want 2 calls, got %d", mock.callCount)
	}
}

func TestRetryTransport_429RetryAfterHeader(t *testing.T) {
	resp429 := mockResp(429)
	resp429.Header.Set("Retry-After", "0") // 0 seconds → no sleep
	mock := &mockTransport{responses: []*http.Response{resp429, mockResp(200)}}
	resp, err := noDelayRT(mock).RoundTrip(newGETRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCalculateBackoff_RetryAfterSecondsCapped(t *testing.T) {
	rt := &RetryTransport{BaseDelay: time.Second}
	tests := []struct {
		header string
		want   time.Duration
	}{
		{header: "0", want: 0},
		{header: "-1", want: 0},
		{header: "1", want: time.Second},
		{header: "29", want: 29 * time.Second},
		{header: "30", want: maxRetryAfter},
		{header: "31", want: maxRetryAfter},
		{header: "86400", want: maxRetryAfter},
	}
	for _, tt := range tests {
		resp := mockResp(429)
		resp.Header.Set("Retry-After", tt.header)
		if got := rt.calculateBackoff(0, resp); got != tt.want {
			t.Errorf("Retry-After %q = %s, want %s", tt.header, got, tt.want)
		}
	}
}

func TestCalculateBackoff_RetryAfterOverflowCaps(t *testing.T) {
	// 9223372037 seconds overflows time.Duration when multiplied by time.Second.
	const header = "9223372037"
	if _, err := strconv.Atoi(header); err != nil {
		t.Skip("platform int cannot represent an overflowing Retry-After")
	}

	rt := &RetryTransport{BaseDelay: time.Second}
	resp := mockResp(429)
	resp.Header.Set("Retry-After", header)
	if got := rt.calculateBackoff(0, resp); got != maxRetryAfter {
		t.Fatalf("overflow Retry-After = %s, want %s", got, maxRetryAfter)
	}
}

func TestCalculateBackoff_RetryAfterHTTPDateCapped(t *testing.T) {
	rt := &RetryTransport{BaseDelay: time.Second}

	far := mockResp(429)
	far.Header.Set("Retry-After", time.Now().Add(2*time.Hour).UTC().Format(http.TimeFormat))
	if got := rt.calculateBackoff(0, far); got != maxRetryAfter {
		t.Fatalf("far HTTP-date = %s, want %s", got, maxRetryAfter)
	}

	past := mockResp(429)
	past.Header.Set("Retry-After", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat))
	if got := rt.calculateBackoff(0, past); got != 0 {
		t.Fatalf("past HTTP-date = %s, want 0", got)
	}

	soon := mockResp(429)
	soon.Header.Set("Retry-After", time.Now().Add(10*time.Second).UTC().Format(http.TimeFormat))
	got := rt.calculateBackoff(0, soon)
	if got <= 0 || got > 11*time.Second || got >= maxRetryAfter {
		t.Fatalf("short HTTP-date = %s, want a delay under 30s", got)
	}
}

func TestCalculateBackoff_InvalidRetryAfterUsesBackoff(t *testing.T) {
	rt := &RetryTransport{BaseDelay: time.Second}
	resp := mockResp(429)
	resp.Header.Set("Retry-After", "not-a-date")
	got := rt.calculateBackoff(0, resp)
	if got < time.Second || got >= time.Second+time.Second/2 {
		t.Fatalf("invalid Retry-After = %s, want backoff in [1s, 1.5s)", got)
	}
}

func TestRetryTransport_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	resp429 := mockResp(429)
	// BaseDelay > 0 so sleep will be triggered; cancelled ctx makes it return error
	mock := &mockTransport{responses: []*http.Response{resp429}}
	rt := &RetryTransport{Base: mock, MaxRetries429: 3, MaxRetries5xx: 1, BaseDelay: time.Hour}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Error("expected error when context is cancelled, got nil")
	}
}
