package zotero

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAllRawChildItemsPaginatesAndPreservesEnvelopes(t *testing.T) {
	const first = `{"key":"ATTACH01","version":"","futureTop":true,"data":{"itemType":"attachment","futureData":1}}`
	const second = `{"key":"ATTACH02","version":4,"data":{"itemType":"attachment"}}`
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/PARENT01/children", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("itemType") != "attachment" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		switch r.URL.Query().Get("start") {
		case "":
			w.Header().Set("Link", `<`+baseURL+`/api/users/0/items/PARENT01/children?itemType=attachment&limit=100&start=1>; rel="next"`)
			w.Header().Set("Backoff", "2")
			_, _ = w.Write([]byte(`[` + first + `]`))
		case "1":
			_, _ = w.Write([]byte(`[` + second + `]`))
		default:
			t.Fatalf("unexpected start %q", r.URL.Query().Get("start"))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	recorder := &sleepRecorder{}
	client := New(srv.URL)
	client.sleep = recorder.sleep
	raw, err := client.AllRawChildItems(context.Background(), UserLibrary(), "PARENT01", ChildrenOptions{ItemType: "attachment", Limit: 100})
	if err != nil {
		t.Fatalf("AllRawChildItems: %v", err)
	}
	if len(raw) != 2 || !reflect.DeepEqual(recorder.waits, []time.Duration{2 * time.Second}) {
		t.Fatalf("raw=%d waits=%v", len(raw), recorder.waits)
	}
	for i, fixture := range []string{first, second} {
		var got, want any
		if err := json.Unmarshal(raw[i], &got); err != nil {
			t.Fatalf("decode raw %d: %v", i, err)
		}
		if err := json.Unmarshal([]byte(fixture), &want); err != nil {
			t.Fatalf("decode fixture %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("raw %d changed: got %#v, want %#v", i, got, want)
		}
	}
}

func TestAllRawChildItemsRejectsInvalidResponses(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: "", want: "empty response body"},
		{name: "null", body: "null", want: "expected a JSON array"},
		{name: "object", body: `{}`, want: "expected a JSON array"},
		{name: "malformed", body: `[`, want: "unexpected end"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			_, err := New(srv.URL).AllRawChildItems(context.Background(), UserLibrary(), "PARENT01", ChildrenOptions{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
