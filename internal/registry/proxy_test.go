package registry

import (
	"net/http"
	"testing"
)

func TestNewRegistryHTTPClient_HonorsProxyEnvironment(t *testing.T) {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(k, "")
	}
	t.Setenv("HTTPS_PROXY", "http://proxy.test:3128")
	t.Setenv("NO_PROXY", "bypass.test")

	tr, ok := NewRegistryHTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("registry client transport is not an *http.Transport")
	}
	if tr.Proxy == nil {
		t.Fatal("registry client transport has no Proxy; HTTPS_PROXY would be ignored")
	}

	req, _ := http.NewRequest(http.MethodGet, DefaultRegistryURL+"/recipes/j/jq.toml", nil)
	got, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy(): %v", err)
	}
	if got == nil || got.String() != "http://proxy.test:3128" {
		t.Errorf("Proxy() for registry URL = %v, want http://proxy.test:3128", got)
	}

	req, _ = http.NewRequest(http.MethodGet, "https://bypass.test/recipes/j/jq.toml", nil)
	got, err = tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy(): %v", err)
	}
	if got != nil {
		t.Errorf("Proxy() for NO_PROXY host = %v, want nil (direct)", got)
	}
}
