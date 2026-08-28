package config

import "testing"

func TestDeleteAPIKeyClearsPlaintext(t *testing.T) {
	cfg := File{APIKey: "PLAINTEXTKEY"}
	// The key is stored plaintext in the config file; removal clears that field.
	if !DeleteAPIKey(&cfg) {
		t.Fatal("expected DeleteAPIKey to report a removed key")
	}
	if cfg.APIKey != "" {
		t.Fatalf("plaintext api_key not cleared: %q", cfg.APIKey)
	}
	// Second call: nothing left to remove.
	if DeleteAPIKey(&cfg) {
		t.Fatal("expected no key to remove on second call")
	}
}

func TestDefaultChainIsEthereumID(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultChain != "1" {
		t.Fatalf("default chain = %q, want 1", cfg.DefaultChain)
	}
}

func TestSetDefaultChainRequiresNumericID(t *testing.T) {
	cfg := File{}
	if err := Set(&cfg, "default_chain=008453"); err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultChain != "8453" {
		t.Fatalf("normalized default chain = %q, want 8453", cfg.DefaultChain)
	}
	for _, value := range []string{"base", "0", "0x2105", ""} {
		if err := Set(&cfg, "default_chain="+value); err == nil {
			t.Errorf("accepted invalid default chain %q", value)
		}
	}
}
