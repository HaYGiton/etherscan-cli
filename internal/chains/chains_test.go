package chains

import (
	"strings"
	"testing"
)

func testRegistry(t *testing.T, extra ...APIChain) *Registry {
	t.Helper()
	rows := []APIChain{
		{DisplayName: "Ethereum Mainnet", ID: "1", Explorer: "https://etherscan.io/", APIURL: "https://api.etherscan.io/v2/api?chainid=1", Status: 1},
		{DisplayName: "Base Mainnet", ID: "8453", Explorer: "https://basescan.org/", APIURL: "https://api.etherscan.io/v2/api?chainid=8453", Status: 1},
		{DisplayName: "Polygon Mainnet", ID: "137", Explorer: "https://polygonscan.com/", APIURL: "https://api.etherscan.io/v2/api?chainid=137", Status: 1},
		{DisplayName: "Arbitrum One Mainnet", ID: "42161", Explorer: "https://arbiscan.io/", APIURL: "https://api.etherscan.io/v2/api?chainid=42161", Status: 1},
	}
	registry, err := New(append(rows, extra...))
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestResolveChain(t *testing.T) {
	registry := testRegistry(t)
	tests := map[string]string{
		"ethereum": "1",
		"eth":      "1",
		"8453":     "8453",
		"base":     "8453",
		"arb":      "42161",
	}
	for input, want := range tests {
		got, err := registry.Resolve(input)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", input, err)
		}
		if got.ID != want {
			t.Fatalf("Resolve(%q)=%s, want %s", input, got.ID, want)
		}
	}
	if _, err := registry.Resolve("not-a-chain"); err == nil {
		t.Fatal("expected unknown chain error")
	}
}

func TestResolveLocalWithoutRegistry(t *testing.T) {
	for input, want := range map[string]string{
		"008453": "8453",
		"base":   "8453",
		"matic":  "137",
		"arb":    "42161",
	} {
		chain, err := ResolveLocal(input)
		if err != nil {
			t.Fatalf("ResolveLocal(%q): %v", input, err)
		}
		if chain.ID != want {
			t.Fatalf("ResolveLocal(%q) ID = %q, want %q", input, chain.ID, want)
		}
	}
	if _, err := ResolveLocal("future-chain"); err == nil || !strings.Contains(err.Error(), "numeric chain ID") {
		t.Fatalf("unknown local name error = %v", err)
	}
	for _, value := range []string{"0", "-1", "0x1", ""} {
		if value == "" {
			continue // empty intentionally defaults to Ethereum mainnet
		}
		if _, ok := NormalizeID(value); ok {
			t.Errorf("NormalizeID(%q) unexpectedly succeeded", value)
		}
	}
}

func TestRegistryNoDuplicatesAndResolvable(t *testing.T) {
	registry := testRegistry(t, APIChain{DisplayName: "Example Chain Mainnet", ID: "999999", Explorer: "https://example.test", APIURL: "https://api.example.test/v2/api?chainid=999999", Status: 2})
	seen := map[string]string{} // id/name/alias -> owning chain name
	seenDisplay := map[string]string{}
	claim := func(key, owner string) {
		if prev, ok := seen[key]; ok {
			t.Fatalf("duplicate identifier %q used by %q and %q", key, prev, owner)
		}
		seen[key] = owner
	}
	for _, c := range registry.All() {
		if c.ID == "" || c.Name == "" || c.DisplayName == "" || c.Explorer == "" {
			t.Fatalf("chain %q missing id/name/display name/explorer", c.Name)
		}
		if previous, ok := seenDisplay[c.DisplayName]; ok {
			t.Fatalf("duplicate display name %q used by %q and %q", c.DisplayName, previous, c.Name)
		}
		seenDisplay[c.DisplayName] = c.Name
		claim(c.ID, c.Name)
		claim(c.Name, c.Name)
		for _, a := range c.Aliases {
			claim(a, c.Name)
		}
		// every chain must resolve by its own id and canonical name
		if got, err := registry.Resolve(c.ID); err != nil || got.ID != c.ID {
			t.Fatalf("Resolve(%q) failed for %q", c.ID, c.Name)
		}
		if got, err := registry.Resolve(c.Name); err != nil || got.ID != c.ID {
			t.Fatalf("Resolve(%q) failed", c.Name)
		}
	}
}

func TestOneWordOfficialNameMayEqualGeneratedSlug(t *testing.T) {
	registry, err := New([]APIChain{{DisplayName: "Gnosis", ID: "100", Explorer: "https://gnosisscan.io", APIURL: "https://api.etherscan.io/v2/api?chainid=100", Status: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if chain, err := registry.Resolve("gnosis"); err != nil || chain.ID != "100" {
		t.Fatalf("resolve one-word chain: chain=%+v err=%v", chain, err)
	}
}

func TestAPIMembershipControlsRegistry(t *testing.T) {
	registry := testRegistry(t, APIChain{DisplayName: "New Network Mainnet", ID: "999999", Explorer: "https://new.example", APIURL: "https://api.new.example/v2/api?chainid=999999", Status: 0, Comment: "maintenance"})
	added, err := registry.Resolve("new-network-mainnet")
	if err != nil || added.ID != "999999" || StatusName(added.Status) != "offline" {
		t.Fatalf("dynamic chain was not built from API data: chain=%+v err=%v", added, err)
	}

	withoutPolygon, err := New([]APIChain{{DisplayName: "Ethereum Mainnet", ID: "1", Explorer: "https://etherscan.io", APIURL: "https://api.etherscan.io/v2/api?chainid=1", Status: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withoutPolygon.Resolve("matic"); err == nil {
		t.Fatal("compatibility alias resolved after its API chain was removed")
	}
}

func TestGeneratedSlugDoesNotChangeWhenMainnetIsAdded(t *testing.T) {
	testnet := APIChain{DisplayName: "Future Chain Testnet", ID: "9001", Explorer: "https://testnet.future.example", APIURL: "https://api.future.example/v2/api?chainid=9001", Status: 1}
	before, err := New([]APIChain{testnet})
	if err != nil {
		t.Fatal(err)
	}
	beforeChain, err := before.Resolve("future-chain-testnet")
	if err != nil {
		t.Fatal(err)
	}
	mainnet := APIChain{DisplayName: "Future Chain Mainnet", ID: "9000", Explorer: "https://future.example", APIURL: "https://api.future.example/v2/api?chainid=9000", Status: 1}
	after, err := New([]APIChain{mainnet, testnet})
	if err != nil {
		t.Fatal(err)
	}
	afterChain, err := after.Resolve("future-chain-testnet")
	if err != nil || afterChain.ID != beforeChain.ID {
		t.Fatalf("testnet slug changed meaning: before=%+v after=%+v err=%v", beforeChain, afterChain, err)
	}
}

func TestLegacySupplementalMetadata(t *testing.T) {
	registry := testRegistry(t)
	base, err := registry.Resolve("base")
	if err != nil {
		t.Fatal(err)
	}
	if base.Symbol != "ETH" || base.FreeTier != FreeTierPaidOnly {
		t.Fatalf("base metadata was not preserved: %+v", base)
	}
	newChain := APIChain{DisplayName: "Unknown Chain Mainnet", ID: "999999", Explorer: "https://unknown.example", APIURL: "https://api.unknown.example/v2/api?chainid=999999", Status: 1}
	dynamic, err := New([]APIChain{newChain})
	if err != nil {
		t.Fatal(err)
	}
	unknown, _ := dynamic.Resolve("unknown-chain-mainnet")
	if unknown.Symbol != "" || unknown.FreeTier != FreeTierUnknown {
		t.Fatalf("new chain metadata should be unknown: %+v", unknown)
	}
}

func TestRegistryRejectsInvalidResponses(t *testing.T) {
	valid := APIChain{DisplayName: "Ethereum Mainnet", ID: "1", Explorer: "https://etherscan.io", APIURL: "https://api.etherscan.io/v2/api?chainid=1", Status: 1}
	for name, rows := range map[string][]APIChain{
		"empty":        nil,
		"missing":      {{DisplayName: "Missing", ID: "2", Explorer: "https://example.test", Status: 1}},
		"duplicate id": {valid, valid},
		"bad status":   {{DisplayName: "Bad", ID: "2", Explorer: "https://example.test", APIURL: "https://api.example.test", Status: 3}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(rows); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// Fallback is the degraded chain list used when the chainlist endpoint is
// unreachable. It must be usable as a registry without any network access.
func TestFallbackRegistry(t *testing.T) {
	registry := Fallback()
	all := registry.All()
	if len(all) == 0 {
		t.Fatal("fallback registry is empty")
	}

	for _, input := range []string{"base", "8453", "0008453", "matic", "137", "ethereum", ""} {
		chain, err := registry.Resolve(input)
		if err != nil {
			t.Fatalf("Resolve(%q) against fallback: %v", input, err)
		}
		if chain.ID == "" || chain.Name == "" {
			t.Fatalf("Resolve(%q) returned an incomplete chain: %+v", input, chain)
		}
	}

	// Liveness is exactly what could not be fetched, so no row may claim a status.
	for _, chain := range all {
		if chain.Status != StatusUnknown {
			t.Fatalf("chain %s has status %d, want StatusUnknown", chain.ID, chain.Status)
		}
		if StatusName(chain.Status) != "unknown" {
			t.Fatalf("chain %s renders status %q, want \"unknown\"", chain.ID, StatusName(chain.Status))
		}
	}

	// A chain added after this release is not in the table; it stays reachable by
	// ID through ResolveLocal, which is what the degraded path resolves with.
	if _, err := registry.Resolve("424242"); err == nil {
		t.Fatal("fallback registry resolved a chain it cannot know about")
	}
	if _, err := ResolveLocal("424242"); err != nil {
		t.Fatalf("ResolveLocal must still accept a numeric ID: %v", err)
	}
}

// Map iteration is randomised, so an unsorted Fallback would reorder the TUI's
// switcher on every launch.
func TestFallbackOrderIsStable(t *testing.T) {
	first := Fallback().All()
	for range 5 {
		next := Fallback().All()
		if len(next) != len(first) {
			t.Fatalf("length changed between calls: %d then %d", len(first), len(next))
		}
		for i := range first {
			if next[i].ID != first[i].ID {
				t.Fatalf("order changed at index %d: %s then %s", i, first[i].ID, next[i].ID)
			}
		}
	}
}
