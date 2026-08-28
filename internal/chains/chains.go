package chains

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

type Chain struct {
	ID          string
	Name        string // stable CLI slug
	DisplayName string // official Etherscan supported-chains name
	Aliases     []string
	Explorer    string
	Symbol      string
	FreeTier    string
	Testnet     bool
	APIURL      string
	Status      int
	Comment     string
}

const (
	FreeTierUnknown   = "unknown"
	FreeTierAvailable = "available"
	FreeTierPaidOnly  = "paid only"
)

// APIChain is one row returned by the Etherscan V2 chain-list endpoint.
type APIChain struct {
	DisplayName string
	ID          string
	Explorer    string
	APIURL      string
	Status      int
	Comment     string
}

// Registry is an immutable snapshot of the chains returned by the API.
type Registry struct {
	chains []Chain
}

type compatibility struct {
	Name         string
	DisplayName  string
	Aliases      []string
	ForceTestnet bool
}

// compatibilityByID preserves inputs accepted by older releases. It does not
// describe supported chains: IDs absent from the API never enter the registry,
// and newly returned IDs work without an entry here.
var compatibilityByID = map[string]compatibility{
	"1":         {Name: "ethereum", Aliases: []string{"eth", "mainnet", "ethereum-mainnet"}},
	"11155111":  {Name: "sepolia", Aliases: []string{"ethereum-sepolia"}},
	"560048":    {Name: "hoodi", Aliases: []string{"hoodi-testnet"}},
	"56":        {Name: "bsc", Aliases: []string{"bnb", "binance", "bnb-smart-chain"}},
	"97":        {Name: "bsc-testnet", Aliases: []string{"bnb-testnet"}},
	"137":       {Name: "polygon", Aliases: []string{"matic", "pol"}},
	"80002":     {Name: "polygon-amoy", Aliases: []string{"amoy"}},
	"8453":      {Name: "base", Aliases: []string{"base-mainnet"}},
	"84532":     {Name: "base-sepolia", Aliases: []string{"basesepolia"}},
	"42161":     {Name: "arbitrum", DisplayName: "Arbitrum One Mainnet", Aliases: []string{"arbitrum-one", "arb"}},
	"421614":    {Name: "arbitrum-sepolia", DisplayName: "Arbitrum Sepolia Testnet", Aliases: []string{"arb-sepolia"}},
	"59144":     {Name: "linea", Aliases: []string{"linea-mainnet"}},
	"59141":     {Name: "linea-sepolia", Aliases: []string{"linea-testnet"}},
	"81457":     {Name: "blast", Aliases: []string{"blast-mainnet"}},
	"168587773": {Name: "blast-sepolia", Aliases: []string{"blast-testnet"}},
	"10":        {Name: "optimism", Aliases: []string{"op", "op-mainnet"}},
	"11155420":  {Name: "optimism-sepolia", Aliases: []string{"op-sepolia"}},
	"43114":     {Name: "avalanche", Aliases: []string{"avax", "avalanche-c-chain"}},
	"43113":     {Name: "avalanche-fuji", Aliases: []string{"fuji"}},
	"199":       {Name: "bttc", Aliases: []string{"bittorrent", "bittorrent-chain"}},
	"1029":      {Name: "bttc-testnet", Aliases: []string{"bittorrent-testnet"}},
	"42220":     {Name: "celo", Aliases: []string{"celo-mainnet"}},
	"11142220":  {Name: "celo-sepolia", Aliases: []string{"celo-testnet"}},
	"252":       {Name: "fraxtal", Aliases: []string{"frax"}},
	"2523":      {Name: "fraxtal-hoodi", Aliases: []string{"fraxtal-testnet"}},
	"100":       {Name: "gnosis", Aliases: []string{"xdai"}},
	"5000":      {Name: "mantle", Aliases: []string{"mantle-mainnet"}},
	"5003":      {Name: "mantle-sepolia", Aliases: []string{"mantle-testnet"}},
	"4352":      {Name: "memecore"},
	"43522":     {Name: "memecore-testnet", Aliases: []string{"memecore-insectarium"}},
	"204":       {Name: "opbnb"},
	"5611":      {Name: "opbnb-testnet"},
	"167000":    {Name: "taiko"},
	"167013":    {Name: "taiko-hoodi", Aliases: []string{"taiko-testnet"}, ForceTestnet: true},
	"50":        {Name: "xdc"},
	"51":        {Name: "xdc-apothem", Aliases: []string{"xdc-testnet", "apothem"}},
	"33139":     {Name: "apechain", Aliases: []string{"ape"}},
	"33111":     {Name: "apechain-curtis", Aliases: []string{"apechain-testnet", "curtis"}},
	"480":       {Name: "world", Aliases: []string{"worldchain"}},
	"4801":      {Name: "world-sepolia", Aliases: []string{"world-testnet"}},
	"146":       {Name: "sonic"},
	"14601":     {Name: "sonic-testnet"},
	"130":       {Name: "unichain", Aliases: []string{"uni"}},
	"1301":      {Name: "unichain-sepolia", Aliases: []string{"unichain-testnet"}},
	"2741":      {Name: "abstract", DisplayName: "Abstract Mainnet"},
	"11124":     {Name: "abstract-sepolia", DisplayName: "Abstract Sepolia Testnet", Aliases: []string{"abstract-testnet"}},
	"80094":     {Name: "berachain", Aliases: []string{"bera"}},
	"80069":     {Name: "berachain-bepolia", Aliases: []string{"berachain-testnet", "bepolia"}},
	"143":       {Name: "monad"},
	"10143":     {Name: "monad-testnet"},
	"999":       {Name: "hyperevm", Aliases: []string{"hyper"}},
	"747474":    {Name: "katana"},
	"737373":    {Name: "katana-bokuto", Aliases: []string{"katana-testnet", "bokuto"}, ForceTestnet: true},
	"1329":      {Name: "sei"},
	"1328":      {Name: "sei-testnet"},
	"988":       {Name: "stable"},
	"2201":      {Name: "stable-testnet"},
	"9745":      {Name: "plasma"},
	"9746":      {Name: "plasma-testnet"},
	"4326":      {Name: "megaeth", Aliases: []string{"mega"}},
	"6343":      {Name: "megaeth-testnet", Aliases: []string{"mega-testnet"}},
}

func New(apiChains []APIChain) (*Registry, error) {
	if len(apiChains) == 0 {
		return nil, fmt.Errorf("unexpected chainlist response: empty result")
	}

	seenIDs := make(map[string]struct{}, len(apiChains))
	seenNames := make(map[string]string, len(apiChains))
	result := make([]Chain, 0, len(apiChains))
	for i, apiChain := range apiChains {
		apiChain.ID = strings.TrimSpace(apiChain.ID)
		apiChain.DisplayName = strings.TrimSpace(apiChain.DisplayName)
		apiChain.Explorer = strings.TrimRight(strings.TrimSpace(apiChain.Explorer), "/")
		apiChain.APIURL = strings.TrimSpace(apiChain.APIURL)
		if apiChain.ID == "" || apiChain.DisplayName == "" || apiChain.Explorer == "" || apiChain.APIURL == "" {
			return nil, fmt.Errorf("unexpected chainlist response: row %d is missing a required field", i+1)
		}
		if _, ok := seenIDs[apiChain.ID]; ok {
			return nil, fmt.Errorf("unexpected chainlist response: duplicate chain ID %s", apiChain.ID)
		}
		seenIDs[apiChain.ID] = struct{}{}
		if apiChain.Status < 0 || apiChain.Status > 2 {
			return nil, fmt.Errorf("unexpected chainlist response: chain %s has invalid status %d", apiChain.ID, apiChain.Status)
		}
		for field, rawURL := range map[string]string{"blockexplorer": apiChain.Explorer, "apiurl": apiChain.APIURL} {
			parsed, err := url.Parse(rawURL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return nil, fmt.Errorf("unexpected chainlist response: chain %s has invalid %s", apiChain.ID, field)
			}
		}

		chain := Chain{
			ID: apiChain.ID, Name: slug(apiChain.DisplayName), DisplayName: apiChain.DisplayName,
			Explorer: apiChain.Explorer, APIURL: apiChain.APIURL, Status: apiChain.Status,
			Comment: apiChain.Comment, FreeTier: FreeTierUnknown,
			Testnet: strings.Contains(strings.ToLower(apiChain.DisplayName), "testnet"),
		}
		if legacy, ok := compatibilityByID[apiChain.ID]; ok {
			if legacy.Name != "" {
				chain.Name = legacy.Name
			}
			chain.Aliases = append([]string(nil), legacy.Aliases...)
			chain.Testnet = chain.Testnet || legacy.ForceTestnet
			chain.Symbol = legacySymbol(apiChain.ID)
			chain.FreeTier = FreeTierAvailable
			if legacyPaidOnly(apiChain.ID) {
				chain.FreeTier = FreeTierPaidOnly
			}
		}
		identifiers := append([]string{chain.ID, chain.Name, strings.ToLower(chain.DisplayName)}, chain.Aliases...)
		for _, identifier := range identifiers {
			identifier = strings.ToLower(strings.TrimSpace(identifier))
			if owner, ok := seenNames[identifier]; ok && owner != chain.ID {
				return nil, fmt.Errorf("unexpected chainlist response: identifier %q is shared by chains %s and %s", identifier, owner, chain.ID)
			}
			seenNames[identifier] = chain.ID
		}
		result = append(result, chain)
	}
	return &Registry{chains: result}, nil
}

func (r *Registry) Resolve(input string) (Chain, error) {
	if r == nil {
		return Chain{}, fmt.Errorf("chain registry is not loaded")
	}
	if strings.TrimSpace(input) == "" {
		input = "1"
	}
	needle := strings.ToLower(strings.TrimSpace(input))
	if id, ok := NormalizeID(needle); ok {
		needle = id
	}
	for _, chain := range r.chains {
		if chain.ID == needle || chain.Name == needle || strings.ToLower(chain.DisplayName) == needle {
			return chain, nil
		}
		for _, alias := range chain.Aliases {
			if alias == needle {
				return chain, nil
			}
		}
	}
	return Chain{}, fmt.Errorf("unknown chain %q", input)
}

// ResolveLocal binds a numeric chain ID or an input supported by an older CLI
// release without consulting the live chain list. New chains must be addressed
// by ID; the target API call remains authoritative for current support.
func ResolveLocal(input string) (Chain, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}
	if id, ok := NormalizeID(input); ok {
		if legacy, exists := compatibilityByID[id]; exists {
			return localCompatibilityChain(id, legacy), nil
		}
		return Chain{ID: id, Name: id, DisplayName: id}, nil
	}
	needle := strings.ToLower(input)
	for id, legacy := range compatibilityByID {
		if legacy.Name == needle {
			return localCompatibilityChain(id, legacy), nil
		}
		for _, alias := range legacy.Aliases {
			if alias == needle {
				return localCompatibilityChain(id, legacy), nil
			}
		}
	}
	return Chain{}, fmt.Errorf("chain must be a numeric chain ID; run 'etherscan chains' to list supported IDs (legacy name %q is not recognized)", input)
}

func localCompatibilityChain(id string, legacy compatibility) Chain {
	displayName := legacy.DisplayName
	if displayName == "" {
		displayName = legacy.Name
	}
	chain := Chain{
		ID: id, Name: legacy.Name, DisplayName: displayName,
		Aliases: append([]string(nil), legacy.Aliases...), Symbol: legacySymbol(id),
		FreeTier: FreeTierAvailable, Testnet: legacy.ForceTestnet,
	}
	if legacyPaidOnly(id) {
		chain.FreeTier = FreeTierPaidOnly
	}
	return chain
}

// NormalizeID accepts positive decimal chain IDs and returns their canonical
// representation without leading zeroes.
func NormalizeID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "", false
	}
	return value, true
}

func (r *Registry) All() []Chain {
	if r == nil {
		return nil
	}
	out := append([]Chain(nil), r.chains...)
	for i := range out {
		out[i].Aliases = append([]string(nil), out[i].Aliases...)
	}
	return out
}

func IsMainnetID(id string) bool {
	return id == "1"
}

func StatusName(status int) string {
	switch status {
	case 0:
		return "offline"
	case 1:
		return "ok"
	case 2:
		return "degraded"
	default:
		return "unknown"
	}
}

func slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func legacyPaidOnly(id string) bool {
	switch id {
	case "56", "97", "8453", "84532", "10", "11155420", "43114", "43113":
		return true
	default:
		return false
	}
}

func legacySymbol(id string) string {
	switch id {
	case "1", "11155111", "560048", "8453", "84532", "42161", "421614", "59144", "59141", "81457", "168587773", "10", "11155420", "167000", "167013", "480", "4801", "130", "1301", "2741", "11124", "4326", "6343":
		return "ETH"
	case "56", "97", "204", "5611":
		return "BNB"
	case "137", "80002":
		return "POL"
	case "43114", "43113":
		return "AVAX"
	case "199", "1029":
		return "BTT"
	case "42220", "11142220":
		return "CELO"
	case "252", "2523":
		return "frxETH"
	case "100":
		return "xDAI"
	case "5000", "5003":
		return "MNT"
	case "50", "51":
		return "XDC"
	case "33139", "33111":
		return "APE"
	case "146", "14601":
		return "S"
	case "80094", "80069":
		return "BERA"
	case "143", "10143":
		return "MON"
	case "999":
		return "HYPE"
	case "1329", "1328":
		return "SEI"
	default:
		return ""
	}
}
