package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/etherscan/etherscan-cli/internal/chains"
)

var testChainRows = []chains.APIChain{
	{DisplayName: "Ethereum Mainnet", ID: "1", Explorer: "https://etherscan.io", APIURL: "https://api.etherscan.io/v2/api?chainid=1", Status: 1},
	{DisplayName: "Sepolia Testnet", ID: "11155111", Explorer: "https://sepolia.etherscan.io", APIURL: "https://api.etherscan.io/v2/api?chainid=11155111", Status: 1},
	{DisplayName: "BNB Smart Chain Mainnet", ID: "56", Explorer: "https://bscscan.com", APIURL: "https://api.etherscan.io/v2/api?chainid=56", Status: 1},
	{DisplayName: "Polygon Mainnet", ID: "137", Explorer: "https://polygonscan.com", APIURL: "https://api.etherscan.io/v2/api?chainid=137", Status: 1},
	{DisplayName: "Base Mainnet", ID: "8453", Explorer: "https://basescan.org", APIURL: "https://api.etherscan.io/v2/api?chainid=8453", Status: 1},
	{DisplayName: "Arbitrum One Mainnet", ID: "42161", Explorer: "https://arbiscan.io", APIURL: "https://api.etherscan.io/v2/api?chainid=42161", Status: 1},
	{DisplayName: "Arbitrum Sepolia Testnet", ID: "421614", Explorer: "https://sepolia.arbiscan.io", APIURL: "https://api.etherscan.io/v2/api?chainid=421614", Status: 1},
	{DisplayName: "Abstract Mainnet", ID: "2741", Explorer: "https://abscan.org", APIURL: "https://api.etherscan.io/v2/api?chainid=2741", Status: 1},
	{DisplayName: "Abstract Sepolia Testnet", ID: "11124", Explorer: "https://sepolia.abscan.org", APIURL: "https://api.etherscan.io/v2/api?chainid=11124", Status: 1},
}

func testCLIRegistry(t *testing.T) *chains.Registry {
	t.Helper()
	registry, err := chains.New(testChainRows)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newChainListServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chainlist" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"comments":"test","totalcount":9,"result":[`+
			`{"chainname":"Ethereum Mainnet","chainid":"1","blockexplorer":"https://etherscan.io","apiurl":"https://api.etherscan.io/v2/api?chainid=1","status":1},`+
			`{"chainname":"Polygon Mainnet","chainid":"137","blockexplorer":"https://polygonscan.com","apiurl":"https://api.etherscan.io/v2/api?chainid=137","status":1}]}`)
	}))
}

func serveTestChainList(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasSuffix(r.URL.Path, "/chainlist") {
		return false
	}
	rows := make([]map[string]any, 0, len(testChainRows))
	for _, row := range testChainRows {
		rows = append(rows, map[string]any{
			"chainname": row.DisplayName, "chainid": row.ID, "blockexplorer": row.Explorer,
			"apiurl": row.APIURL, "status": row.Status, "comment": row.Comment,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"comments": "test", "totalcount": len(rows), "result": rows})
	return true
}
