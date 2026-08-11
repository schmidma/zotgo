package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const importedAttachmentEnvelope = `{
	"key":"ATTACH01",
	"version":"",
	"futureTop":{"kept":true},
	"links":{
		"enclosure":{
			"href":"http://127.0.0.1:23119/api/users/0/items/ATTACH01/file/view",
			"type":"application/pdf",
			"title":"paper.pdf",
			"length":1234,
			"futureLink":true
		}
	},
	"data":{
		"key":"ATTACH01",
		"version":"",
		"itemType":"attachment",
		"parentItem":"PARENT01",
		"title":"Full Text PDF",
		"linkMode":"imported_file",
		"contentType":"application/pdf",
		"charset":"",
		"filename":"paper.pdf",
		"url":"https://example.com/paper.pdf",
		"accessDate":"2026-01-01T00:00:00Z",
		"dateAdded":"2026-01-02T00:00:00Z",
		"dateModified":"2026-01-03T00:00:00Z",
		"tags":[{"tag":"review","type":0}],
		"md5":"0123456789abcdef",
		"mtime":1700000000000,
		"futureData":7
	}
}`

func attachmentServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/{key}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("key") {
		case "ATTACH01":
			_, _ = w.Write([]byte(importedAttachmentEnvelope))
		case "LINKURL1":
			_, _ = w.Write([]byte(`{"key":"LINKURL1","data":{"itemType":"attachment","parentItem":false,"title":"Website","linkMode":"linked_url","contentType":"text/html","url":"https://example.com","tags":[],"md5":null,"mtime":null}}`))
		case "NOLENGTH":
			_, _ = w.Write([]byte(`{"key":"NOLENGTH","links":{"enclosure":{"href":"http://local/file","type":"application/pdf","title":"missing.pdf"}},"data":{"itemType":"attachment","linkMode":"imported_file","filename":"missing.pdf","tags":[],"md5":null,"mtime":null}}`))
		case "WRONG001":
			_, _ = w.Write([]byte(`{"key":"WRONG001","data":{"itemType":"book","title":"Not an attachment"}}`))
		case "MALFORM1":
			_, _ = w.Write([]byte(`{"key":"MALFORM1","data":{"itemType":"attachment","mtime":{}}}`))
		case "BADKEY01":
			_, _ = w.Write([]byte(`{"key":"OTHER001","data":{"itemType":"attachment"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /api/users/0/items/{key}/file", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("attachment show requested file bytes: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	return httptest.NewServer(mux)
}

func TestAttachmentShowHelpAndHuman(t *testing.T) {
	srv := attachmentServer(t)
	defer srv.Close()

	rootHelp, _, err := runCLI(srv.URL, "--help")
	if err != nil {
		t.Fatalf("root help: %v", err)
	}
	if !strings.Contains(rootHelp, "attachment") {
		t.Fatalf("root help missing attachment:\n%s", rootHelp)
	}
	showHelp, _, err := runCLI(srv.URL, "attachment", "show", "--help")
	if err != nil {
		t.Fatalf("attachment show help: %v", err)
	}
	for _, want := range []string{"<attachment-key>", "conservative file status", "without opening", "does not claim"} {
		if !strings.Contains(showHelp, want) {
			t.Errorf("show help missing %q:\n%s", want, showHelp)
		}
	}

	out, _, err := runCLI(srv.URL, "attachment", "show", "ATTACH01")
	if err != nil {
		t.Fatalf("attachment show: %v", err)
	}
	for _, want := range []string{
		"ATTACH01", "PARENT01", "Full Text PDF", "imported_file", "application/pdf", "paper.pdf",
		"0123456789abcdef", "1700000000000", "1234", "metadata-available", "size metadata",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "file exists") {
		t.Fatalf("human output overclaimed file existence:\n%s", out)
	}
}

func TestAttachmentShowJSON(t *testing.T) {
	srv := attachmentServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--json", "attachment", "show", "ATTACH01")
	if err != nil {
		t.Fatalf("attachment show JSON: %v", err)
	}
	var doc struct {
		Schema int            `json:"schema"`
		Kind   string         `json:"kind"`
		Data   map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if doc.Schema != 2 || doc.Kind != "attachment" || doc.Data["key"] != "ATTACH01" || doc.Data["parentKey"] != "PARENT01" {
		t.Fatalf("document = %#v", doc)
	}
	if len(doc.Data) != 16 {
		t.Fatalf("attachment fields = %v", doc.Data)
	}
	enclosure, ok := doc.Data["enclosure"].(map[string]any)
	if !ok || enclosure["length"] != float64(1234) || enclosure["title"] != "paper.pdf" {
		t.Fatalf("enclosure = %#v", doc.Data["enclosure"])
	}
	status, ok := doc.Data["fileStatus"].(map[string]any)
	if !ok || status["state"] != "metadata-available" {
		t.Fatalf("file status = %#v", doc.Data["fileStatus"])
	}
	for _, forbidden := range []string{`"version"`, "futureTop", "futureData", "futureLink"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("stable JSON leaked %q:\n%s", forbidden, out)
		}
	}
}

func TestAttachmentShowJSONL(t *testing.T) {
	srv := attachmentServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--jsonl", "attachment", "show", "ATTACH01")
	if err != nil {
		t.Fatalf("attachment show JSONL: %v", err)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("JSONL has multiple lines:\n%s", out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSONL: %v", err)
	}
	if doc["schema"] != float64(2) || doc["kind"] != "attachment" {
		t.Fatalf("JSONL document = %#v", doc)
	}
}

func TestAttachmentShowRawPreservesEnvelope(t *testing.T) {
	srv := attachmentServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--raw", "attachment", "show", "ATTACH01")
	if err != nil {
		t.Fatalf("attachment show raw: %v", err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if err := json.Unmarshal([]byte(importedAttachmentEnvelope), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw changed envelope:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAttachmentShowConservativeStatuses(t *testing.T) {
	srv := attachmentServer(t)
	defer srv.Close()

	for _, tt := range []struct {
		key   string
		state string
	}{
		{key: "LINKURL1", state: "not-applicable"},
		{key: "NOLENGTH", state: "location-advertised"},
	} {
		out, _, err := runCLI(srv.URL, "--json", "attachment", "show", tt.key)
		if err != nil {
			t.Fatalf("%s: %v", tt.key, err)
		}
		if !strings.Contains(out, `"state": "`+tt.state+`"`) {
			t.Errorf("%s output missing state %q:\n%s", tt.key, tt.state, out)
		}
	}
}

func TestAttachmentShowInputErrors(t *testing.T) {
	srv := attachmentServer(t)
	defer srv.Close()

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing argument", args: []string{"attachment", "show"}, want: "missing attachment key"},
		{name: "not found", args: []string{"attachment", "show", "MISSING1"}, want: `no attachment with key "MISSING1"`},
		{name: "wrong type", args: []string{"attachment", "show", "WRONG001"}, want: `type "book", not attachment`},
		{name: "malformed", args: []string{"attachment", "show", "MALFORM1"}, want: "mtime"},
		{name: "response key mismatch", args: []string{"attachment", "show", "BADKEY01"}, want: `response has key "OTHER001"`},
		{name: "raw wrong type", args: []string{"--raw", "attachment", "show", "WRONG001"}, want: `type "book", not attachment`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI(srv.URL, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAttachmentShowGroupLibrary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/groups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":42,"data":{"id":42,"name":"Research Group"}}]`))
	})
	mux.HandleFunc("GET /api/groups/42/items/GROUPATT", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"key":"GROUPATT","data":{"itemType":"attachment","linkMode":"linked_file","tags":[],"md5":null,"mtime":null}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "-L", "Research Group", "--json", "attachment", "show", "GROUPATT")
	if err != nil {
		t.Fatalf("group attachment show: %v", err)
	}
	for _, want := range []string{`"type": "group"`, `"id": 42`, `"state": "linked-unverified"`} {
		if !strings.Contains(out, want) {
			t.Errorf("group output missing %q:\n%s", want, out)
		}
	}
}

func TestAttachmentShowWebProfile(t *testing.T) {
	t.Setenv("ZOTGO_API_KEY", "test-key")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /keys/current", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key")
		}
		_, _ = w.Write([]byte(`{"userID":77,"username":"ada","access":{"user":{"library":true}}}`))
	})
	mux.HandleFunc("GET /users/77/items/WEBATT01", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key")
		}
		_, _ = w.Write([]byte(`{"key":"WEBATT01","links":{"enclosure":{"href":"https://api.zotero.org/users/77/items/WEBATT01/file","type":"application/pdf","title":"web.pdf"}},"data":{"itemType":"attachment","linkMode":"imported_file","filename":"web.pdf","tags":[],"md5":"abcdef","mtime":1700000000000}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--web", "--json", "attachment", "show", "WEBATT01")
	if err != nil {
		t.Fatalf("web attachment show: %v", err)
	}
	if !strings.Contains(out, `"key": "WEBATT01"`) || !strings.Contains(out, `"state": "location-advertised"`) {
		t.Fatalf("web output = %s", out)
	}
}
