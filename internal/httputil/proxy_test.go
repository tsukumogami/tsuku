package httputil

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// proxyTestEnv is a local stand-in for a network that needs a proxy. It runs
// a TLS origin server and a proxy that handles both CONNECT (for https) and
// plain forward requests (for http), and records what the proxy was asked
// for. No real network is used: every dial the client makes is routed to one
// of the two local servers, and each dial target is recorded.
type proxyTestEnv struct {
	origin *httptest.Server
	proxy  *httptest.Server

	mu       sync.Mutex
	proxied  []string // CONNECT targets and forward-request URLs the proxy saw
	directTo []string // dial targets that bypassed the proxy
}

func newProxyTestEnv(t *testing.T) *proxyTestEnv {
	t.Helper()
	env := &proxyTestEnv{}

	env.origin = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect-to-http" {
			http.Redirect(w, r, "http://proxied.test/plain", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "origin:"+r.Host)
	}))
	t.Cleanup(env.origin.Close)

	env.proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			env.record(&env.proxied, "CONNECT "+r.Host)
			upstream, err := net.Dial("tcp", env.origin.Listener.Addr().String())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				_ = upstream.Close()
				return
			}
			_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
			go func() { _, _ = io.Copy(upstream, conn); _ = upstream.Close() }()
			go func() { _, _ = io.Copy(conn, upstream); _ = conn.Close() }()
			return
		}
		// Forward proxy: a plain http request arrives with an absolute URL.
		env.record(&env.proxied, "GET "+r.URL.String())
		_, _ = io.WriteString(w, "proxy-forwarded:"+r.URL.Host)
	}))
	t.Cleanup(env.proxy.Close)

	// Only the proxy's own address is ever proxy-bound; clear every variant
	// so the developer's or CI runner's environment can't leak in.
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(k, "")
	}
	t.Setenv("HTTPS_PROXY", env.proxy.URL)
	t.Setenv("HTTP_PROXY", env.proxy.URL)
	t.Setenv("NO_PROXY", "bypass.test")
	return env
}

func (e *proxyTestEnv) record(list *[]string, s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	*list = append(*list, s)
}

func (e *proxyTestEnv) snapshot() (proxied, direct []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.proxied...), append([]string(nil), e.directTo...)
}

// client builds the production client and changes only what the test needs:
// dials go to the local servers, and the origin's test certificate is trusted.
// The transport's Proxy field, the thing under test, is left as built.
func (e *proxyTestEnv) client(t *testing.T) *http.Client {
	t.Helper()
	client := NewSecureClient(DefaultOptions())
	tr := client.Transport.(*http.Transport)

	proxyAddr := e.proxy.Listener.Addr().String()
	originAddr := e.origin.Listener.Addr().String()
	var d net.Dialer
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == proxyAddr {
			return d.DialContext(ctx, network, addr)
		}
		e.record(&e.directTo, addr)
		return d.DialContext(ctx, network, originAddr)
	}

	tlsCfg := e.origin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tlsCfg.ServerName = "example.com" // name on httptest's certificate
	tr.TLSClientConfig = tlsCfg
	return client
}

func get(t *testing.T, client *http.Client, url string) (string, error) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func TestNewSecureClient_HonorsHTTPSProxy(t *testing.T) {
	env := newProxyTestEnv(t)

	body, err := get(t, env.client(t), "https://proxied.test/file")
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	if body != "origin:proxied.test" {
		t.Errorf("body = %q, want %q", body, "origin:proxied.test")
	}

	proxied, direct := env.snapshot()
	if len(proxied) != 1 || proxied[0] != "CONNECT proxied.test:443" {
		t.Errorf("proxy saw %q, want exactly [CONNECT proxied.test:443]", proxied)
	}
	if len(direct) != 0 {
		t.Errorf("client dialed %q directly, want no direct dials", direct)
	}
}

func TestNewSecureClient_HonorsHTTPProxy(t *testing.T) {
	env := newProxyTestEnv(t)

	body, err := get(t, env.client(t), "http://proxied.test/file")
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	if body != "proxy-forwarded:proxied.test" {
		t.Errorf("body = %q, want %q", body, "proxy-forwarded:proxied.test")
	}

	proxied, direct := env.snapshot()
	if len(proxied) != 1 || proxied[0] != "GET http://proxied.test/file" {
		t.Errorf("proxy saw %q, want exactly [GET http://proxied.test/file]", proxied)
	}
	if len(direct) != 0 {
		t.Errorf("client dialed %q directly, want no direct dials", direct)
	}
}

func TestNewSecureClient_NoProxyHostBypassesProxy(t *testing.T) {
	env := newProxyTestEnv(t)

	body, err := get(t, env.client(t), "https://bypass.test/file")
	if err != nil {
		t.Fatalf("GET to NO_PROXY host: %v", err)
	}
	if body != "origin:bypass.test" {
		t.Errorf("body = %q, want %q", body, "origin:bypass.test")
	}

	proxied, direct := env.snapshot()
	if len(proxied) != 0 {
		t.Errorf("proxy saw %q, want nothing for a NO_PROXY host", proxied)
	}
	if len(direct) != 1 || direct[0] != "bypass.test:443" {
		t.Errorf("direct dials = %q, want exactly [bypass.test:443]", direct)
	}
}

func TestNewSecureClient_RedirectToHTTPBlockedThroughProxy(t *testing.T) {
	env := newProxyTestEnv(t)

	_, err := get(t, env.client(t), "https://proxied.test/redirect-to-http")
	if err == nil {
		t.Fatal("expected the https -> http redirect to be refused")
	}
	if !strings.Contains(err.Error(), "redirect to non-HTTPS URL is not allowed") {
		t.Errorf("error = %v, want the non-HTTPS redirect refusal", err)
	}

	// The first hop went through the proxy; the refused http hop must never
	// have been sent, through the proxy or around it.
	proxied, direct := env.snapshot()
	if len(proxied) != 1 || proxied[0] != "CONNECT proxied.test:443" {
		t.Errorf("proxy saw %q, want exactly [CONNECT proxied.test:443]", proxied)
	}
	if len(direct) != 0 {
		t.Errorf("client dialed %q directly, want no direct dials", direct)
	}
}
