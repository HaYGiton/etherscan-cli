package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/etherscan/etherscan-cli/internal/config"
)

// A bare `etherscan` invocation prints the Quick Start guide (and does not launch
// the explorer or run an update check).
func TestBareInvocationPrintsQuickStart(t *testing.T) {
	manager := &fakeUpdateManager{}
	root := newRootCommand(BuildInfo{Version: "1.1.0"}, manager)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{"Quick Start", "etherscan login", "docs.etherscan.io"} {
		if !strings.Contains(got, want) {
			t.Errorf("bare invocation output missing %q; got:\n%s", want, got)
		}
	}
	// A bare invocation must not trigger the update flow.
	if manager.upgradedVersion != "" || manager.skipped != "" {
		t.Errorf("bare invocation should not touch the updater, got %+v", manager)
	}
}

func TestWhoamiDoesNotFetchChainList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := config.Save(config.File{DefaultChain: "base", APIKey: "TESTKEY"}); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	root := newRootCommand(BuildInfo{}, &fakeUpdateManager{})
	root.SetArgs([]string{"--base-url", server.URL, "whoami"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatalf("whoami made %d network requests", requests)
	}
}

func TestChainsToleratesMalformedConfig(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	configPath := filepath.Join(configRoot, "etherscan", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("not = [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newChainListServer(t)
	defer server.Close()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	originalStdout := os.Stdout
	os.Stdout = devNull
	defer func() { os.Stdout = originalStdout }()

	root := newRootCommand(BuildInfo{}, &fakeUpdateManager{})
	root.SetArgs([]string{"--base-url", server.URL + "/v2/api", "chains"})
	if err := root.Execute(); err != nil {
		t.Fatalf("chains failed with malformed config: %v", err)
	}
}

func TestOrdinaryCommandsDoNotFetchChainList(t *testing.T) {
	for _, chainInput := range []string{"8453", "base", "matic"} {
		t.Run(chainInput, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if strings.HasSuffix(r.URL.Path, "/chainlist") {
					t.Error("ordinary command fetched chain list")
				}
				wantID := "8453"
				if chainInput == "matic" {
					wantID = "137"
				}
				if got := r.URL.Query().Get("chainid"); got != wantID {
					t.Errorf("chainid = %q, want %q", got, wantID)
				}
				_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":"0"}`))
			}))
			defer server.Close()

			devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer devNull.Close()
			originalStdout := os.Stdout
			os.Stdout = devNull
			defer func() { os.Stdout = originalStdout }()

			root := newRootCommand(BuildInfo{}, &fakeUpdateManager{})
			root.SetArgs([]string{"--api-key", "TESTKEY", "--base-url", server.URL + "/v2/api", "--chain", chainInput, "account", "balance", "0x0000000000000000000000000000000000000000"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want one API call", requests)
			}
		})
	}
}

// whoami must agree with the resolution every other command performs: before
// this was fixed it echoed the raw flag/env/config string, so an unresolvable
// chain was reported as active and the command exited 0.
func TestWhoamiRejectsUnresolvableChain(t *testing.T) {
	for _, source := range []string{"flag", "env", "config"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			cfg := config.File{DefaultChain: "1", APIKey: "TESTKEY"}
			args := []string{"whoami"}
			switch source {
			case "flag":
				args = []string{"--chain", "future-chain", "whoami"}
			case "env":
				t.Setenv("ETHERSCAN_CHAIN", "future-chain")
			case "config":
				cfg.DefaultChain = "future-chain"
			}
			if _, err := config.Save(cfg); err != nil {
				t.Fatal(err)
			}

			root := newRootCommand(BuildInfo{}, &fakeUpdateManager{})
			root.SetArgs(args)
			err := root.Execute()
			if err == nil {
				t.Fatal("whoami accepted an unresolvable chain")
			}
			if !strings.Contains(err.Error(), "numeric chain ID") {
				t.Fatalf("error = %v, want the ResolveLocal guidance", err)
			}
		})
	}
}

// whoami reports the resolved chain, not the raw input: legacy names and
// non-canonical IDs both have to surface the chain ID requests will carry.
func TestWhoamiReportsResolvedChain(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"base", "chain:   base (8453)"},
		{"0008453", "chain:   base (8453)"},
		{"matic", "chain:   polygon (137)"},
		{"424242", "chain:   424242"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			if _, err := config.Save(config.File{DefaultChain: "1", APIKey: "TESTKEY"}); err != nil {
				t.Fatal(err)
			}
			capture := filepath.Join(t.TempDir(), "stdout")
			file, err := os.Create(capture)
			if err != nil {
				t.Fatal(err)
			}
			originalStdout := os.Stdout
			os.Stdout = file
			root := newRootCommand(BuildInfo{}, &fakeUpdateManager{})
			root.SetArgs([]string{"--chain", tc.input, "whoami"})
			execErr := root.Execute()
			os.Stdout = originalStdout
			file.Close()
			if execErr != nil {
				t.Fatal(execErr)
			}

			got, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), tc.want) {
				t.Fatalf("whoami output missing %q; got:\n%s", tc.want, got)
			}
		})
	}
}

// The TUI degrades to the compiled-in chain list, but `etherscan chains` must
// not: its distinguishing output is the live status/comment, and reporting a
// cached "ok" for a chain that is currently offline is worse than an error,
// because callers act on it.
func TestChainsDoesNotFallBackOnOutage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "chainlist unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	capture := filepath.Join(t.TempDir(), "stdout")
	file, err := os.Create(capture)
	if err != nil {
		t.Fatal(err)
	}
	originalStdout := os.Stdout
	os.Stdout = file
	root := newRootCommand(BuildInfo{}, &fakeUpdateManager{})
	root.SetArgs([]string{"--base-url", server.URL + "/v2/api", "chains"})
	execErr := root.Execute()
	os.Stdout = originalStdout
	file.Close()

	if execErr == nil {
		t.Fatal("chains succeeded while the chain list was unreachable")
	}
	if !strings.Contains(execErr.Error(), "load supported chains") {
		t.Fatalf("error = %v, want the load failure", execErr)
	}
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "ethereum") {
		t.Fatalf("chains printed a stale list instead of failing:\n%s", got)
	}
}
