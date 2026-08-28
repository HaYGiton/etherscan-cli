package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/etherscan/etherscan-cli/internal/client"
	"github.com/etherscan/etherscan-cli/internal/config"
	"github.com/etherscan/etherscan-cli/internal/output"
)

// TestRuntimeRequiresKey guards the API-key-required policy: with no key from
// flag, env, or config, runtime() must fail fast with errNoAPIKey instead of
// building a client that sends keyless (server-throttled) requests.
func TestRuntimeRequiresKey(t *testing.T) {
	// Isolate from the developer's real env and config file.
	t.Setenv("ETHERSCAN_API_KEY", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	state := &globalState{timeout: 5 * time.Second, rate: 3}
	_, err := runtime(context.Background(), state)
	if !errors.Is(err, errNoAPIKey) {
		t.Fatalf("expected errNoAPIKey, got %v", err)
	}

	// The --api-key flag alone must satisfy the requirement.
	server := newChainListServer(t)
	defer server.Close()
	state.baseURL = server.URL + "/v2/api"
	state.apiKey = "TESTKEY"
	if _, err := runtime(context.Background(), state); err != nil {
		t.Fatalf("runtime with --api-key failed: %v", err)
	}

	// So must the environment variable.
	state.apiKey = ""
	t.Setenv("ETHERSCAN_API_KEY", "ENVKEY")
	if _, err := runtime(context.Background(), state); err != nil {
		t.Fatalf("runtime with env key failed: %v", err)
	}
}

func TestBuildRuntimeAllowsEmptyKeyForTUI(t *testing.T) {
	t.Setenv("ETHERSCAN_API_KEY", "")
	server := newChainListServer(t)
	defer server.Close()
	state := &globalState{timeout: 5 * time.Second, rate: 3, baseURL: server.URL + "/v2/api"}
	rt, err := buildRuntime(context.Background(), state, config.File{}, "", true)
	if err != nil {
		t.Fatalf("keyless TUI runtime failed: %v", err)
	}
	if rt.client == nil || rt.chain.ID != "1" {
		t.Fatalf("incomplete keyless runtime: client=%v chain=%+v", rt.client, rt.chain)
	}
}

func TestRebindRuntimeChainPreservesSession(t *testing.T) {
	registry := testCLIRegistry(t)
	ethereum, err := registry.Resolve("ethereum")
	if err != nil {
		t.Fatal(err)
	}
	rt := resolvedRuntime{
		client:   client.New(client.Options{BaseURL: "https://example.test/v2/api", APIKey: "TESTKEY", ChainID: ethereum.ID, RateLimit: 3}),
		format:   output.JSON,
		chain:    ethereum,
		registry: registry,
	}
	originalClient := rt.client

	chain, err := rebindRuntimeChain(&rt, "matic")
	if err != nil {
		t.Fatalf("rebindRuntimeChain(matic): %v", err)
	}
	if chain.ID != "137" || rt.chain.ID != chain.ID || rt.chain.Name != chain.Name {
		t.Fatalf("chain not rebound to polygon: %+v", rt.chain)
	}
	if rt.client == originalClient {
		t.Fatal("chain rebind must use an immutable client clone")
	}
	if rt.format != output.JSON {
		t.Fatalf("chain rebind changed output format to %q", rt.format)
	}

	currentClient, currentChainID := rt.client, rt.chain.ID
	if _, err := rebindRuntimeChain(&rt, "not-a-chain"); err == nil {
		t.Fatal("expected unknown-chain error")
	}
	if rt.client != currentClient || rt.chain.ID != currentChainID {
		t.Fatal("failed chain rebind mutated the runtime")
	}
}

func TestTUIRuntimeFetchesRegistryOnce(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !serveTestChainList(w, r) {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	state := &globalState{timeout: 5 * time.Second, rate: 1000, baseURL: server.URL + "/v2/api"}
	rt, err := buildRuntime(context.Background(), state, config.File{}, "TESTKEY", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rebindRuntimeChain(&rt, "matic"); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("chain list requests = %d, want 1", requests)
	}
}

func TestLegacyChainNameBypassesRegistryFetch(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()
	state := &globalState{chain: "matic", timeout: 5 * time.Second, rate: 1000, baseURL: server.URL + "/v2/api"}
	rt, err := buildRuntime(context.Background(), state, config.File{}, "TESTKEY", false)
	if err != nil {
		t.Fatal(err)
	}
	if rt.chain.ID != "137" || rt.registry != nil || requests != 0 {
		t.Fatalf("legacy runtime fetched registry or resolved incorrectly: chain=%+v registry=%v requests=%d", rt.chain, rt.registry, requests)
	}
}

func TestUnknownChainNameFailsWithoutRegistryFetch(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	state := &globalState{chain: "future-chain", timeout: 5 * time.Second, rate: 1000, baseURL: server.URL + "/v2/api"}
	_, err := buildRuntime(context.Background(), state, config.File{}, "TESTKEY", false)
	if err == nil || !strings.Contains(err.Error(), "numeric chain ID") {
		t.Fatalf("error = %v, want numeric chain ID guidance", err)
	}
	if requests != 0 {
		t.Fatalf("unknown name made %d requests", requests)
	}
}

func TestNumericChainBypassesRegistryFetch(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()
	state := &globalState{chain: "008453", timeout: 5 * time.Second, rate: 1000, baseURL: server.URL + "/v2/api"}
	rt, err := buildRuntime(context.Background(), state, config.File{}, "TESTKEY", false)
	if err != nil {
		t.Fatal(err)
	}
	if rt.chain.ID != "8453" || rt.registry != nil || requests != 0 {
		t.Fatalf("numeric runtime fetched registry or resolved incorrectly: chain=%+v registry=%v requests=%d", rt.chain, rt.registry, requests)
	}
}

func TestResolveKeyPrecedence(t *testing.T) {
	t.Setenv("ETHERSCAN_API_KEY", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Flag beats everything.
	t.Setenv("ETHERSCAN_API_KEY", "ENVKEY")
	if got := resolveKey(&globalState{apiKey: "FLAGKEY"}, cfg); got != "FLAGKEY" {
		t.Fatalf("flag should win, got %q", got)
	}
	// Env beats config.
	cfg.APIKey = "CFGKEY"
	if got := resolveKey(&globalState{}, cfg); got != "ENVKEY" {
		t.Fatalf("env should beat config, got %q", got)
	}
	// Config is the last resort.
	t.Setenv("ETHERSCAN_API_KEY", "")
	if got := resolveKey(&globalState{}, cfg); got != "CFGKEY" {
		t.Fatalf("config fallback failed, got %q", got)
	}
	// Nothing set -> empty.
	cfg.APIKey = ""
	if got := resolveKey(&globalState{}, cfg); got != "" {
		t.Fatalf("expected empty key, got %q", got)
	}
}
