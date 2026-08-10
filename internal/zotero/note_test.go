package zotero

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func noteEnvelope(key, parent, modified, html string) Envelope {
	data, _ := json.Marshal(map[string]any{
		"itemType":     "note",
		"parentItem":   parent,
		"dateAdded":    "2026-01-01T00:00:00Z",
		"dateModified": modified,
		"tags":         []map[string]any{{"tag": "review", "type": 0}},
		"note":         html,
	})
	return Envelope{Key: key, Data: data}
}

func TestEnvelopeNote(t *testing.T) {
	note, err := noteEnvelope("NOTE0001", "PARENT01", "2026-02-01T00:00:00Z", `<div><p>Body</p></div>`).Note()
	if err != nil {
		t.Fatalf("Note: %v", err)
	}
	want := Note{
		Key:          "NOTE0001",
		ParentKey:    "PARENT01",
		DateAdded:    "2026-01-01T00:00:00Z",
		DateModified: "2026-02-01T00:00:00Z",
		Tags:         []Tag{{Tag: "review", Type: 0}},
		HTML:         `<div><p>Body</p></div>`,
	}
	if !reflect.DeepEqual(note, want) {
		t.Fatalf("Note = %#v, want %#v", note, want)
	}
}

func TestEnvelopeStandaloneNoteParentShapes(t *testing.T) {
	for _, parent := range []string{"false", "null", ""} {
		data := `{"itemType":"note","note":"","tags":[]`
		if parent != "" {
			data += `,"parentItem":` + parent
		}
		data += `}`
		note, err := (Envelope{Key: "NOTE0001", Data: json.RawMessage(data)}).Note()
		if err != nil {
			t.Fatalf("parent %q: %v", parent, err)
		}
		if note.ParentKey != "" {
			t.Errorf("parent %q decoded as %q", parent, note.ParentKey)
		}
		if note.Tags == nil {
			t.Errorf("parent %q produced nil tags", parent)
		}
	}
}

func TestNotesModifiedDescendingOrder(t *testing.T) {
	envelopes := []Envelope{
		noteEnvelope("NOTE0003", "PARENT01", "", ""),
		noteEnvelope("NOTE0002", "PARENT01", "2026-02-01T00:00:00Z", "body"),
		noteEnvelope("NOTE0001", "PARENT01", "2026-02-01T00:00:00Z", "body"),
		noteEnvelope("NOTE0000", "PARENT01", "2026-03-01T00:00:00Z", "body"),
	}
	notes, err := Notes(envelopes)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	var keys []string
	for _, note := range notes {
		keys = append(keys, note.Key)
	}
	want := []string{"NOTE0000", "NOTE0001", "NOTE0002", "NOTE0003"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("ordered keys = %v, want %v", keys, want)
	}
}

func TestNoteRejectsMalformedItems(t *testing.T) {
	tests := []struct {
		name string
		item Envelope
		want string
	}{
		{name: "missing key", item: Envelope{Data: json.RawMessage(`{"itemType":"note","note":""}`)}, want: "missing note key"},
		{name: "missing data", item: Envelope{Key: "NOTE0001"}, want: "has no data"},
		{name: "invalid data", item: Envelope{Key: "NOTE0001", Data: json.RawMessage(`{`)}, want: "unexpected end"},
		{name: "wrong type", item: Envelope{Key: "NOTE0001", Data: json.RawMessage(`{"itemType":"book"}`)}, want: `type "book"`},
		{name: "bad parent", item: Envelope{Key: "NOTE0001", Data: json.RawMessage(`{"itemType":"note","parentItem":{},"note":""}`)}, want: "parentItem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.item.Note()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Note error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAllRawNoteChildrenPagination(t *testing.T) {
	const first = `{"key":"NOTE0001","futureTop":{"kept":1},"data":{"itemType":"note","parentItem":"PARENT01","dateAdded":"2026-01-01T00:00:00Z","dateModified":"2026-01-02T00:00:00Z","tags":[],"note":"<div>one</div>","futureData":true}}`
	const second = `{"key":"NOTE0002","futureTop":{"kept":2},"data":{"itemType":"note","parentItem":"PARENT01","dateAdded":"2026-01-03T00:00:00Z","dateModified":"2026-01-04T00:00:00Z","tags":[],"note":"<div>two</div>"}}`

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/PARENT01/children", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("itemType") != "note" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if r.Header.Get("Zotero-API-Version") != "3" {
			t.Error("missing API version header")
		}
		switch r.URL.Query().Get("start") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/users/0/items/PARENT01/children?itemType=note&limit=100&start=1>; rel="next"`, srv.URL))
			_, _ = fmt.Fprintf(w, "[%s]", first)
		case "1":
			_, _ = fmt.Fprintf(w, "[%s]", second)
		default:
			t.Errorf("unexpected start %q", r.URL.Query().Get("start"))
		}
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	client := New(srv.URL)
	opts := ChildrenOptions{ItemType: "note", Limit: 100}
	raw, err := client.AllRawChildItems(context.Background(), UserLibrary(), "PARENT01", opts)
	if err != nil {
		t.Fatalf("AllRawChildItems: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("raw length = %d", len(raw))
	}
	for i, fixture := range []string{first, second} {
		var got, want any
		if err := json.Unmarshal(raw[i], &got); err != nil {
			t.Fatalf("decode raw[%d]: %v", i, err)
		}
		if err := json.Unmarshal([]byte(fixture), &want); err != nil {
			t.Fatalf("decode fixture[%d]: %v", i, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("raw[%d] changed envelope: got %#v, want %#v", i, got, want)
		}
	}
}

func TestAllRawChildItemsRejectsInvalidCollectionBodies(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: "", want: "empty response body"},
		{name: "null", body: "null", want: "expected a JSON array"},
		{name: "object", body: `{}`, want: "expected a JSON array"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/users/0/items/PARENT01/children", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			_, err := New(srv.URL).AllRawChildItems(context.Background(), UserLibrary(), "PARENT01", ChildrenOptions{ItemType: "note"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAllRawChildItemsHonorsBackoff(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/PARENT01/children", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") == "" {
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/users/0/items/PARENT01/children?itemType=note&limit=100&start=1>; rel="next"`, base))
			w.Header().Set("Backoff", "1")
			_, _ = w.Write([]byte(`[{"key":"NOTE0001"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"key":"NOTE0002"}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	client := New(srv.URL)
	recorder := &sleepRecorder{}
	client.sleep = recorder.sleep
	items, err := client.AllRawChildItems(context.Background(), UserLibrary(), "PARENT01", ChildrenOptions{ItemType: "note", Limit: 100})
	if err != nil {
		t.Fatalf("AllRawChildItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if len(recorder.waits) != 1 || recorder.waits[0] != time.Second {
		t.Fatalf("waits = %v, want one 1s backoff", recorder.waits)
	}
}

func TestRawItemPreservesNoteEnvelope(t *testing.T) {
	const fixture = `{"key":"NOTE0001","futureTop":{"kept":true},"data":{"itemType":"note","parentItem":false,"note":"<div>body</div>","futureData":7}}`
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/NOTE0001", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fixture))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	raw, err := New(srv.URL).RawItem(context.Background(), UserLibrary(), "NOTE0001")
	if err != nil {
		t.Fatalf("RawItem: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode raw item: %v", err)
	}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw item changed envelope: got %#v, want %#v", got, want)
	}
}
