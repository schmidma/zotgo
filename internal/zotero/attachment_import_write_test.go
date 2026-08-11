package zotero

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateItemsReturningKeysIgnoresUnrelatedEnvelopeFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Zotero-Server-ID", "SERVERID1234")
	})
	mux.HandleFunc("POST /api/users/0/items", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "key" {
			t.Error("missing local key")
		}
		_, _ = w.Write([]byte(`{
			"successful":{"0":{"key":"ATTACH01","version":"","links":{"up":false},"future":true}},
			"success":{"0":"ATTACH01"},"unchanged":{},"failed":{}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := New(srv.URL)
	client.SetLocalKey("key")
	result, err := client.CreateItemsReturningKeys(context.Background(), UserLibrary(), []json.RawMessage{
		json.RawMessage(`{"itemType":"attachment","linkMode":"imported_file"}`),
	})
	if err != nil {
		t.Fatalf("CreateItemsReturningKeys: %v", err)
	}
	if len(result.Successful) != 1 || result.Successful["0"] != "ATTACH01" || len(result.Failed) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCreateItemsReturningKeysRejectsMissingKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Zotero-Server-ID", "SERVERID1234")
	})
	mux.HandleFunc("POST /api/users/0/items", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"successful":{"0":{"links":{"up":false}}},"failed":{}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := New(srv.URL)
	client.SetLocalKey("key")
	_, err := client.CreateItemsReturningKeys(context.Background(), UserLibrary(), []json.RawMessage{json.RawMessage(`{}`)})
	if err == nil || !strings.Contains(err.Error(), "has no key") {
		t.Fatalf("error = %v", err)
	}
}

func TestLibraryVersionDoesNotDecodeItemEnvelopes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "1" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Header().Set("Last-Modified-Version", "47")
		w.Header().Set("Zotero-Server-ID", "SERVERID1234")
		_, _ = w.Write([]byte(`[{"key":"ITEM0001","version":"","links":{"up":false}}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := New(srv.URL)
	version, err := client.LibraryVersion(context.Background(), UserLibrary())
	if err != nil {
		t.Fatalf("LibraryVersion: %v", err)
	}
	if version != 47 || client.ServerID() != "SERVERID1234" {
		t.Fatalf("version/server = %d/%q", version, client.ServerID())
	}
}
