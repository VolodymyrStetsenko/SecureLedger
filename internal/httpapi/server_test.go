package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VolodymyrStetsenko/secureledger/internal/app"
	"github.com/VolodymyrStetsenko/secureledger/internal/store/memory"
)

func TestTransferReplayReturns200(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New(app.New(memory.New(), nil, app.Config{}), logger)
	server := httptest.NewServer(handler)
	defer server.Close()

	alice := createAccountHTTP(t, server.URL, "alice", 1000)
	bob := createAccountHTTP(t, server.URL, "bob", 0)

	body := map[string]any{
		"from_account_id": alice,
		"to_account_id":   bob,
		"amount_minor":    100,
	}
	first := postJSON(t, server.URL+"/v1/transfers", body, map[string]string{
		"X-Principal-ID": "alice", "X-Principal-Role": "customer", "Idempotency-Key": "http-key-12345678",
	})
	defer first.Body.Close()
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first status=%d", first.StatusCode)
	}
	second := postJSON(t, server.URL+"/v1/transfers", body, map[string]string{
		"X-Principal-ID": "alice", "X-Principal-Role": "customer", "Idempotency-Key": "http-key-12345678",
	})
	defer second.Body.Close()
	if second.StatusCode != http.StatusOK || second.Header.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay status=%d header=%q", second.StatusCode, second.Header.Get("Idempotent-Replayed"))
	}
}

func TestJSONBoundaryRejectsUnsafeInputs(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(New(app.New(memory.New(), nil, app.Config{}), logger))
	defer server.Close()

	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{name: "missing content type", body: `{}`, wantStatus: http.StatusUnsupportedMediaType},
		{name: "unknown field", contentType: "application/json", body: `{"owner_id":"alice","currency":"GBP","unexpected":true}`, wantStatus: http.StatusBadRequest},
		{name: "multiple values", contentType: "application/json", body: `{}` + `{}`, wantStatus: http.StatusBadRequest},
		{name: "too large", contentType: "application/json", body: `{"owner_id":"` + strings.Repeat("a", (1<<20)+1) + `","currency":"GBP"}`, wantStatus: http.StatusRequestEntityTooLarge},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/accounts", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			req.Header.Set("X-Principal-ID", "operator")
			req.Header.Set("X-Principal-Role", "operator")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, tc.wantStatus, body)
			}
		})
	}
}

func TestInvalidListLimitReturns400(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(New(app.New(memory.New(), nil, app.Config{}), nil))
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/audit?limit=invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Principal-ID", "auditor")
	req.Header.Set("X-Principal-Role", "auditor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestAccountAndOversightHTTPFlows(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(New(app.New(memory.New(), nil, app.Config{}), nil))
	defer server.Close()

	health, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK || health.Header.Get("Cache-Control") != "no-store" || health.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected health response: status=%d headers=%v", health.StatusCode, health.Header)
	}

	alice := createAccountHTTP(t, server.URL, "alice", 2_000_000)
	bob := createAccountHTTP(t, server.URL, "bob", 0)

	owned := getHTTP(t, server.URL+"/v1/accounts/"+alice, map[string]string{
		"X-Principal-ID": "alice", "X-Principal-Role": "customer",
	})
	defer owned.Body.Close()
	if owned.StatusCode != http.StatusOK {
		t.Fatalf("owner account status=%d", owned.StatusCode)
	}

	forbidden := getHTTP(t, server.URL+"/v1/accounts/"+alice, map[string]string{
		"X-Principal-ID": "bob", "X-Principal-Role": "customer",
	})
	defer forbidden.Body.Close()
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner account status=%d", forbidden.StatusCode)
	}

	missing := getHTTP(t, server.URL+"/v1/accounts/missing", map[string]string{
		"X-Principal-ID": "operator", "X-Principal-Role": "operator",
	})
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing account status=%d", missing.StatusCode)
	}

	transfer := postJSON(t, server.URL+"/v1/transfers", map[string]any{
		"from_account_id": alice, "to_account_id": bob, "amount_minor": 1_000_000,
	}, map[string]string{
		"X-Principal-ID": "alice", "X-Principal-Role": "customer", "Idempotency-Key": "risk-http-key",
	})
	defer transfer.Body.Close()
	if transfer.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(transfer.Body)
		t.Fatalf("transfer status=%d body=%s", transfer.StatusCode, body)
	}

	for _, path := range []string{"/v1/journal", "/v1/audit", "/v1/risk-events"} {
		resp := getHTTP(t, server.URL+path+"?limit=10", map[string]string{
			"X-Principal-ID": "auditor", "X-Principal-Role": "auditor",
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status=%d", path, resp.StatusCode)
		}
	}

	limit := postJSON(t, server.URL+"/v1/transfers", map[string]any{
		"from_account_id": alice, "to_account_id": bob, "amount_minor": 100_000_001,
	}, map[string]string{
		"X-Principal-ID": "alice", "X-Principal-Role": "customer", "Idempotency-Key": "limit-http-key",
	})
	defer limit.Body.Close()
	if limit.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("limit status=%d", limit.StatusCode)
	}
}

func createAccountHTTP(t *testing.T, base, owner string, balance int64) string {
	t.Helper()
	resp := postJSON(t, base+"/v1/accounts", map[string]any{
		"owner_id": owner, "currency": "GBP", "opening_balance_minor": balance,
	}, map[string]string{"X-Principal-ID": "operator", "X-Principal-Role": "operator"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create account status=%d body=%s", resp.StatusCode, b)
	}
	var account struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatal(err)
	}
	return account.ID
}

func postJSON(t *testing.T, url string, body any, headers map[string]string) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getHTTP(t *testing.T, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
