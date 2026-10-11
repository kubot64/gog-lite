package googleauth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const (
	callbackHoldEnv        = "GOG_LITE_OAUTH_CALLBACK_HOLD"
	callbackHoldTimeoutEnv = "GOG_LITE_OAUTH_CALLBACK_HOLD_MS"
	callbackHoldArg        = "oauth-callback-hold"
	callbackHoldFD         = 3
)

// callbackHoldTimeout is how long the detached listener keeps the port.
// Tests shorten it. Production matches the manual-state TTL.
var callbackHoldTimeout = manualStateTTL

const callbackHoldPage = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<title>gog-lite</title>
</head>
<body>
<p>認証リダイレクトを受け取りました。アドレスバーの URL をコピーし、ターミナルで --auth-url に渡してください。</p>
</body>
</html>
`

// RunCallbackHolderIfRequested serves the loopback OAuth redirect when this
// process was spawned as the port holder. The normal CLI returns handled=false.
// Both the internal env and the argv marker are required, so a leaked env var
// does not turn a normal invocation into the holder.
func RunCallbackHolderIfRequested() (bool, error) {
	if os.Getenv(callbackHoldEnv) != "1" || len(os.Args) < 2 || os.Args[1] != callbackHoldArg {
		return false, nil
	}

	f := os.NewFile(callbackHoldFD, "oauth-callback")
	if f == nil {
		return true, errors.New("missing oauth callback socket")
	}
	defer f.Close()

	ln, err := net.FileListener(f)
	if err != nil {
		return true, fmt.Errorf("adopt oauth callback socket: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), callbackHoldTimeoutFromEnv())
	defer cancel()

	if err := serveCallback(ctx, ln); err != nil {
		return true, err
	}

	return true, nil
}

func callbackHoldTimeoutFromEnv() time.Duration {
	raw := os.Getenv(callbackHoldTimeoutEnv)
	if raw == "" {
		return manualStateTTL
	}

	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms <= 0 {
		return manualStateTTL
	}

	return time.Duration(ms) * time.Millisecond
}

// callbackProbeTimeout bounds the loopback check that distinguishes this
// program's holder from another process bound to the same port.
const callbackProbeTimeout = 2 * time.Second

// callbackProbeClient requests only the saved loopback redirect. It does not
// use a proxy and does not follow redirects.
var callbackProbeClient = &http.Client{
	Timeout: callbackProbeTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: time.Second,
		}).DialContext,
		ResponseHeaderTimeout:  time.Second,
		MaxResponseHeaderBytes: 8 << 10,
		DisableKeepAlives:      true,
		DisableCompression:     true,
	},
}

// ensureCallbackPort reports whether redirectURI can still be used.
// keep is true when this call started the holder, when the process already
// bound to the port returns the holder page, or when this platform cannot
// hold the port and the port is free. keep is false when another process
// has the port and does not return the holder page; the caller drops the
// saved state.
func ensureCallbackPort(redirectURI string) (bool, error) {
	host, err := callbackHost(redirectURI)
	if err != nil {
		return false, err
	}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", host)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return callbackHolderResponding(redirectURI), nil
		}

		return false, fmt.Errorf("listen for oauth callback: %w", err)
	}

	if err := holdCallbackPortOrRelease(ln); err != nil {
		_ = ln.Close()
		return false, err
	}

	return true, nil
}

// callbackHolderResponding reports whether redirectURI is served by the
// callback holder. Only that process returns callbackHoldPage.
func callbackHolderResponding(redirectURI string) bool {
	if _, err := callbackHost(redirectURI); err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), callbackProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirectURI, nil)
	if err != nil {
		return false
	}
	req.Close = true

	resp, err := callbackProbeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(len(callbackHoldPage))+1))
	if err != nil {
		return false
	}

	return resp.StatusCode == http.StatusOK && bytes.Equal(body, []byte(callbackHoldPage))
}

func callbackHost(redirectURI string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "/oauth2/callback" {
		return "", fmt.Errorf("invalid loopback redirect URI")
	}

	return net.JoinHostPort(u.Hostname(), u.Port()), nil
}

// holdCallbackPortOrRelease keeps ln open in a detached child on platforms
// that can pass the socket. Elsewhere it closes ln and returns nil, so step 1
// still succeeds: the copy page does not load, and the user copies the
// address-bar URL. A failure to hold on a supported platform is returned.
func holdCallbackPortOrRelease(ln net.Listener) error {
	if !callbackHoldSupported {
		if err := ln.Close(); err != nil {
			return fmt.Errorf("close oauth callback listener: %w", err)
		}

		return nil
	}

	return holdCallbackPort(ln)
}

// holdCallbackPort passes ln to a detached child and waits until the port
// accepts connections. Step 1 then exits, so the listener has to outlive it.
// The child keeps the port until the manual-state TTL. Closing it after the
// first request would let another process bind the port and take a later redirect.
func holdCallbackPort(ln net.Listener) error {
	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		return errors.New("oauth callback listener is not tcp")
	}

	addr := ln.Addr().String()

	f, err := tcp.File()
	if err != nil {
		return fmt.Errorf("dup oauth callback listener: %w", err)
	}
	defer f.Close()

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	cmd := exec.Command(exe, callbackHoldArg)
	// Leave stdio nil so the child uses os.DevNull. It must not inherit the
	// terminal, where a log line could include the redirect URL and its code.
	cmd.Env = []string{
		callbackHoldEnv + "=1",
		callbackHoldTimeoutEnv + "=" + strconv.FormatInt(callbackHoldTimeout.Milliseconds(), 10),
	}
	if path := os.Getenv("PATH"); path != "" {
		cmd.Env = append(cmd.Env, "PATH="+path)
	}
	cmd.ExtraFiles = []*os.File{f}
	cmd.SysProcAttr = callbackHolderSysProcAttr()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start oauth callback holder: %w", err)
	}

	if err := ln.Close(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("close parent oauth listener: %w", err)
	}

	if err := waitUntilAccepting(addr, cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	_ = cmd.Process.Release()

	return nil
}

func waitUntilAccepting(addr string, cmd *exec.Cmd) error {
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error

	for {
		if holderExited(cmd) {
			return errors.New("oauth callback holder exited before listening")
		}

		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("oauth callback holder did not accept connections: %w", lastErr)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

func serveCallback(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           http.HandlerFunc(handleOAuthCallback),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}

	return err
}

func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/oauth2/callback" {
		http.NotFound(w, r)
		return
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host != "127.0.0.1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// The authorization code stays in the browser address bar. Do not copy
	// the query string into the body, logs, or the state file.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.WriteHeader(http.StatusOK)

	if r.Method == http.MethodHead {
		return
	}

	_, _ = io.WriteString(w, callbackHoldPage)
}
