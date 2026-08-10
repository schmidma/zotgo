package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const olderNoteEnvelope = `{
	"key":"NOTE0001",
	"version":8,
	"futureTop":{"kept":1},
	"data":{
		"itemType":"note",
		"parentItem":"PARENT01",
		"dateAdded":"2026-01-01T00:00:00Z",
		"dateModified":"2026-01-02T00:00:00Z",
		"tags":[{"tag":"review","type":0}],
		"note":"<div><p>PRIVATE OLDER BODY</p></div>",
		"futureData":true
	}
}`

const newerNoteEnvelope = `{
	"key":"NOTE0002",
	"version":9,
	"futureTop":{"kept":2},
	"data":{
		"itemType":"note",
		"parentItem":"PARENT01",
		"dateAdded":"2026-02-01T00:00:00Z",
		"dateModified":"2026-02-02T00:00:00Z",
		"tags":[],
		"note":"<div><p>PRIVATE NEWER BODY</p></div>"
	}
}`

const getNoteEnvelope = `{
	"key":"NOTEGET1",
	"version":10,
	"futureTop":{"kept":true},
	"data":{
		"itemType":"note",
		"parentItem":false,
		"dateAdded":"2026-03-01T00:00:00Z",
		"dateModified":"2026-03-02T00:00:00Z",
		"tags":[{"tag":"requested","type":0}],
		"note":"<div data-schema-version=\"9\"><p>EXPLICIT REQUESTED BODY</p></div>",
		"futureData":7
	}
}`

func noteServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/{key}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("key") {
		case "PARENT01", "MALFORM1":
			_, _ = fmt.Fprintf(w, `{"key":%q,"data":{"key":%q,"itemType":"journalArticle"}}`, r.PathValue("key"), r.PathValue("key"))
		case "EMPTY001":
			_, _ = w.Write([]byte(`{"key":"EMPTY001","data":{"key":"EMPTY001","itemType":"book"}}`))
		case "ATTACH01":
			_, _ = w.Write([]byte(`{"key":"ATTACH01","data":{"key":"ATTACH01","itemType":"attachment"}}`))
		case "NOTEGET1":
			_, _ = w.Write([]byte(getNoteEnvelope))
		case "EMPTYNOTE":
			_, _ = w.Write([]byte(`{"key":"EMPTYNOTE","data":{"itemType":"note","parentItem":null,"dateAdded":"","dateModified":"","tags":[],"note":""}}`))
		case "WRONG001":
			_, _ = w.Write([]byte(`{"key":"WRONG001","data":{"itemType":"book","title":"Not a note"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /api/users/0/items/{key}/children", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("itemType"); got != "note" {
			t.Errorf("itemType = %q, want note", got)
		}
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("limit = %q, want 100", got)
		}
		switch r.PathValue("key") {
		case "PARENT01":
			switch r.URL.Query().Get("start") {
			case "":
				w.Header().Set("Link", fmt.Sprintf(`<%s/api/users/0/items/PARENT01/children?itemType=note&limit=100&start=1>; rel="next"`, srv.URL))
				_, _ = fmt.Fprintf(w, "[%s]", olderNoteEnvelope)
			case "1":
				_, _ = fmt.Fprintf(w, "[%s]", newerNoteEnvelope)
			default:
				t.Errorf("unexpected start %q", r.URL.Query().Get("start"))
			}
		case "EMPTY001":
			_, _ = w.Write([]byte(`[]`))
		case "MALFORM1":
			_, _ = w.Write([]byte(`[{"key":"BADNOTE1","data":{"itemType":"note","parentItem":"OTHER001","tags":[],"note":"body"}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv = httptest.NewServer(mux)
	return srv
}

func TestNoteHelpAndListHuman(t *testing.T) {
	srv := noteServer(t)
	defer srv.Close()

	rootHelp, _, err := runCLI(srv.URL, "--help")
	if err != nil {
		t.Fatalf("root help: %v", err)
	}
	if !strings.Contains(rootHelp, "note") {
		t.Fatalf("root help missing note:\n%s", rootHelp)
	}
	listHelp, _, err := runCLI(srv.URL, "note", "list", "--help")
	if err != nil {
		t.Fatalf("note list help: %v", err)
	}
	for _, want := range []string{"<item-key>", "without their bodies", "note get", "--raw"} {
		if !strings.Contains(listHelp, want) {
			t.Errorf("list help missing %q:\n%s", want, listHelp)
		}
	}
	getHelp, _, err := runCLI(srv.URL, "note", "get", "--help")
	if err != nil {
		t.Fatalf("note get help: %v", err)
	}
	if !strings.Contains(getHelp, "<note-key>") || !strings.Contains(getHelp, "rich HTML") {
		t.Fatalf("get help incomplete:\n%s", getHelp)
	}

	out, _, err := runCLI(srv.URL, "note", "list", "PARENT01")
	if err != nil {
		t.Fatalf("note list: %v", err)
	}
	for _, want := range []string{"KEY", "MODIFIED", "CONTENT", "TAGS", "NOTE0001", "NOTE0002", "2 notes"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "NOTE0002") > strings.Index(out, "NOTE0001") {
		t.Fatalf("human notes not modified-descending:\n%s", out)
	}
	for _, forbidden := range []string{"PRIVATE OLDER BODY", "PRIVATE NEWER BODY", "<div>"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("human list leaked %q:\n%s", forbidden, out)
		}
	}
}

func TestNoteListJSONAndJSONL(t *testing.T) {
	srv := noteServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--json", "note", "list", "PARENT01")
	if err != nil {
		t.Fatalf("note list JSON: %v", err)
	}
	var doc struct {
		Schema int              `json:"schema"`
		Kind   string           `json:"kind"`
		Data   []map[string]any `json:"data"`
		Meta   struct {
			Shown int `json:"shown"`
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if doc.Schema != 2 || doc.Kind != "notes" || doc.Meta.Shown != 2 || doc.Meta.Total != 2 {
		t.Fatalf("document = %#v", doc)
	}
	if doc.Data[0]["key"] != "NOTE0002" || doc.Data[1]["key"] != "NOTE0001" {
		t.Fatalf("data order = %#v", doc.Data)
	}
	listFields := map[string]bool{"key": true, "parentKey": true, "dateAdded": true, "dateModified": true, "tags": true, "hasContent": true}
	for _, record := range doc.Data {
		if len(record) != len(listFields) {
			t.Errorf("list fields = %v", record)
		}
		for field := range record {
			if !listFields[field] {
				t.Errorf("unexpected list field %q", field)
			}
		}
	}
	if strings.Contains(out, "PRIVATE") || strings.Contains(out, `"html"`) || strings.Contains(out, `"version"`) {
		t.Fatalf("stable list leaked raw/body data:\n%s", out)
	}

	jsonl, _, err := runCLI(srv.URL, "--jsonl", "note", "list", "PARENT01")
	if err != nil {
		t.Fatalf("note list JSONL: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(jsonl), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSONL lines = %d:\n%s", len(lines), jsonl)
	}
	for i, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if record["schema"] != float64(2) || record["kind"] != "note" {
			t.Fatalf("line %d = %#v", i, record)
		}
	}
}

func TestNoteListRawPreservesEnvelopes(t *testing.T) {
	srv := noteServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--raw", "note", "list", "PARENT01")
	if err != nil {
		t.Fatalf("note list raw: %v", err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	fixture := fmt.Sprintf("[%s,%s]", olderNoteEnvelope, newerNoteEnvelope)
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw list changed envelopes:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNoteGetHumanJSONJSONLAndRaw(t *testing.T) {
	srv := noteServer(t)
	defer srv.Close()

	human, _, err := runCLI(srv.URL, "note", "get", "NOTEGET1")
	if err != nil {
		t.Fatalf("note get human: %v", err)
	}
	for _, want := range []string{"NOTEGET1", "2026-03-01", "requested", "HTML:", "EXPLICIT REQUESTED BODY"} {
		if !strings.Contains(human, want) {
			t.Errorf("human get missing %q:\n%s", want, human)
		}
	}

	stable, _, err := runCLI(srv.URL, "--json", "note", "get", "NOTEGET1")
	if err != nil {
		t.Fatalf("note get JSON: %v", err)
	}
	var doc struct {
		Schema int            `json:"schema"`
		Kind   string         `json:"kind"`
		Data   map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(stable), &doc); err != nil {
		t.Fatalf("decode get JSON: %v", err)
	}
	if doc.Schema != 2 || doc.Kind != "note" || doc.Data["html"] != `<div data-schema-version="9"><p>EXPLICIT REQUESTED BODY</p></div>` || doc.Data["parentKey"] != "" {
		t.Fatalf("get document = %#v", doc)
	}
	if len(doc.Data) != 7 {
		t.Fatalf("get fields = %v", doc.Data)
	}

	jsonl, _, err := runCLI(srv.URL, "--jsonl", "note", "get", "NOTEGET1")
	if err != nil {
		t.Fatalf("note get JSONL: %v", err)
	}
	if strings.Count(strings.TrimSpace(jsonl), "\n") != 0 || !strings.Contains(jsonl, `"kind":"note"`) || !strings.Contains(jsonl, "EXPLICIT REQUESTED BODY") {
		t.Fatalf("get JSONL = %s", jsonl)
	}

	raw, _, err := runCLI(srv.URL, "--raw", "note", "get", "NOTEGET1")
	if err != nil {
		t.Fatalf("note get raw: %v", err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if err := json.Unmarshal([]byte(getNoteEnvelope), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw get changed envelope:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNoteEmptyAndInputErrors(t *testing.T) {
	srv := noteServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--json", "note", "list", "EMPTY001")
	if err != nil {
		t.Fatalf("empty list JSON: %v", err)
	}
	if !strings.Contains(out, `"data": []`) || !strings.Contains(out, `"shown": 0`) {
		t.Fatalf("empty list JSON = %s", out)
	}
	human, _, err := runCLI(srv.URL, "note", "list", "EMPTY001")
	if err != nil || human != "No notes for item EMPTY001.\n" {
		t.Fatalf("empty list human = %q, %v", human, err)
	}
	raw, _, err := runCLI(srv.URL, "--raw", "note", "list", "EMPTY001")
	if err != nil || strings.TrimSpace(raw) != "[]" {
		t.Fatalf("empty list raw = %q, %v", raw, err)
	}

	emptyGet, _, err := runCLI(srv.URL, "--json", "note", "get", "EMPTYNOTE")
	if err != nil {
		t.Fatalf("empty note get: %v", err)
	}
	if !strings.Contains(emptyGet, `"html": ""`) || !strings.Contains(emptyGet, `"hasContent": false`) {
		t.Fatalf("empty note get = %s", emptyGet)
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "list missing argument", args: []string{"note", "list"}, want: "missing item key"},
		{name: "list parent missing", args: []string{"note", "list", "MISSING1"}, want: `no item with key "MISSING1"`},
		{name: "list attachment parent", args: []string{"note", "list", "ATTACH01"}, want: `type "attachment", not a bibliographic item`},
		{name: "list wrong child parent", args: []string{"note", "list", "MALFORM1"}, want: `belongs to "OTHER001"`},
		{name: "get missing argument", args: []string{"note", "get"}, want: "missing note key"},
		{name: "get missing", args: []string{"note", "get", "MISSING1"}, want: `no note with key "MISSING1"`},
		{name: "get wrong type", args: []string{"note", "get", "WRONG001"}, want: `type "book", not note`},
		{name: "raw get wrong type", args: []string{"--raw", "note", "get", "WRONG001"}, want: `type "book", not note`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI(srv.URL, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestNoteListGroupLibrary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/groups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":42,"data":{"id":42,"name":"Research Group"}}]`))
	})
	mux.HandleFunc("GET /api/groups/42/items/GROUPPAR", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"key":"GROUPPAR","data":{"itemType":"report"}}`))
	})
	mux.HandleFunc("GET /api/groups/42/items/GROUPPAR/children", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"key":"GROUPNOT","data":{"itemType":"note","parentItem":"GROUPPAR","dateAdded":"","dateModified":"","tags":[],"note":"group body"}}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "-L", "Research Group", "--json", "note", "list", "GROUPPAR")
	if err != nil {
		t.Fatalf("group note list: %v", err)
	}
	for _, want := range []string{`"type": "group"`, `"id": 42`, `"parentKey": "GROUPPAR"`} {
		if !strings.Contains(out, want) {
			t.Errorf("group output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "group body") {
		t.Fatalf("group list leaked body: %s", out)
	}
}

func TestNoteGetWebProfile(t *testing.T) {
	t.Setenv("ZOTGO_API_KEY", "test-key")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /keys/current", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key")
		}
		_, _ = w.Write([]byte(`{"userID":77,"username":"ada","access":{"user":{"library":true}}}`))
	})
	mux.HandleFunc("GET /users/77/items/WEBNOTE1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key")
		}
		_, _ = w.Write([]byte(`{"key":"WEBNOTE1","data":{"itemType":"note","parentItem":false,"dateAdded":"","dateModified":"","tags":[],"note":"web body"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--web", "--json", "note", "get", "WEBNOTE1")
	if err != nil {
		t.Fatalf("web note get: %v", err)
	}
	if !strings.Contains(out, `"key": "WEBNOTE1"`) || !strings.Contains(out, `"html": "web body"`) {
		t.Fatalf("web output = %s", out)
	}
}
