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

const annotationLaterEnvelope = `{
	"key":"ANN00002",
	"version":9,
	"futureTop":{"kept":2},
	"data":{
		"itemType":"annotation",
		"parentItem":"ATTACH01",
		"annotationType":"note",
		"annotationComment":"PRIVATE COMMENT BODY",
		"annotationColor":"#2ea8e5",
		"annotationPageLabel":"13",
		"annotationSortIndex":"00013|00002",
		"annotationPosition":{"pageIndex":12},
		"futureData":"preserved"
	}
}`

const annotationEarlierEnvelope = `{
	"key":"ANN00001",
	"version":8,
	"futureTop":{"kept":1},
	"data":{
		"itemType":"annotation",
		"parentItem":"ATTACH01",
		"annotationType":"highlight",
		"annotationText":"PRIVATE TEXT BODY",
		"annotationColor":"#ffd400",
		"annotationPageLabel":"12",
		"annotationSortIndex":"00012|00001",
		"annotationPosition":{"pageIndex":11},
		"futureData":true
	}
}`

func annotationServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/{key}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("key") {
		case "ATTACH01", "EMPTY001", "MALFORM1":
			_, _ = fmt.Fprintf(w, `{"key":%q,"data":{"key":%q,"itemType":"attachment","title":"PDF"}}`, r.PathValue("key"), r.PathValue("key"))
		case "PARENT01":
			_, _ = w.Write([]byte(`{"key":"PARENT01","data":{"key":"PARENT01","itemType":"journalArticle","title":"Paper"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /api/users/0/items/{key}/children", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("itemType"); got != "annotation" {
			t.Errorf("itemType = %q, want annotation", got)
		}
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("limit = %q, want 100", got)
		}
		switch r.PathValue("key") {
		case "ATTACH01":
			switch r.URL.Query().Get("start") {
			case "":
				w.Header().Set("Total-Results", "2")
				w.Header().Set("Link", fmt.Sprintf(`<%s/api/users/0/items/ATTACH01/children?itemType=annotation&limit=100&start=1>; rel="next"`, srv.URL))
				_, _ = fmt.Fprintf(w, "[%s]", annotationLaterEnvelope)
			case "1":
				w.Header().Set("Total-Results", "2")
				_, _ = fmt.Fprintf(w, "[%s]", annotationEarlierEnvelope)
			default:
				t.Errorf("unexpected start %q", r.URL.Query().Get("start"))
			}
		case "EMPTY001":
			w.Header().Set("Total-Results", "0")
			_, _ = w.Write([]byte(`[]`))
		case "MALFORM1":
			w.Header().Set("Total-Results", "1")
			_, _ = w.Write([]byte(`[{"key":"BADANN01","data":{"itemType":"annotation","parentItem":"MALFORM1"}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv = httptest.NewServer(mux)
	return srv
}

func TestAnnotationListHumanAndHelp(t *testing.T) {
	srv := annotationServer(t)
	defer srv.Close()

	rootHelp, _, err := runCLI(srv.URL, "--help")
	if err != nil {
		t.Fatalf("root help: %v", err)
	}
	if !strings.Contains(rootHelp, "annotation") {
		t.Fatalf("root help missing annotation:\n%s", rootHelp)
	}
	listHelp, _, err := runCLI(srv.URL, "annotation", "list", "--help")
	if err != nil {
		t.Fatalf("annotation list help: %v", err)
	}
	for _, want := range []string{"<attachment-key>", "document order", "--raw"} {
		if !strings.Contains(listHelp, want) {
			t.Errorf("list help missing %q:\n%s", want, listHelp)
		}
	}

	out, _, err := runCLI(srv.URL, "annotation", "list", "ATTACH01")
	if err != nil {
		t.Fatalf("annotation list: %v", err)
	}
	for _, want := range []string{"KEY", "PAGE", "SORT INDEX", "TYPE", "TEXT", "COMMENT", "ANN00001", "ANN00002", "00012|00001", "00013|00002", "highlight", "note", "2 annotations"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "ANN00001") > strings.Index(out, "ANN00002") {
		t.Fatalf("human output is not in document order:\n%s", out)
	}
	for _, forbidden := range []string{"PRIVATE TEXT BODY", "PRIVATE COMMENT BODY", "pageIndex"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("human output leaked %q:\n%s", forbidden, out)
		}
	}
}

func TestAnnotationListJSON(t *testing.T) {
	srv := annotationServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--json", "annotation", "list", "ATTACH01")
	if err != nil {
		t.Fatalf("annotation list JSON: %v", err)
	}
	var doc struct {
		Schema int    `json:"schema"`
		Kind   string `json:"kind"`
		Data   []struct {
			Key           string `json:"key"`
			AttachmentKey string `json:"attachmentKey"`
			Type          string `json:"type"`
			SortIndex     string `json:"sortIndex"`
			HasText       bool   `json:"hasText"`
			HasComment    bool   `json:"hasComment"`
		} `json:"data"`
		Meta struct {
			Shown int `json:"shown"`
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if doc.Schema != 2 || doc.Kind != "annotations" || doc.Meta.Shown != 2 || doc.Meta.Total != 2 {
		t.Fatalf("document header/meta = %#v", doc)
	}
	if len(doc.Data) != 2 || doc.Data[0].Key != "ANN00001" || doc.Data[1].Key != "ANN00002" {
		t.Fatalf("data order = %#v", doc.Data)
	}
	if doc.Data[0].AttachmentKey != "ATTACH01" || doc.Data[0].Type != "highlight" || !doc.Data[0].HasText || doc.Data[0].HasComment {
		t.Fatalf("first annotation = %#v", doc.Data[0])
	}
	if doc.Data[1].HasText || !doc.Data[1].HasComment {
		t.Fatalf("second annotation = %#v", doc.Data[1])
	}
	for _, forbidden := range []string{"PRIVATE TEXT BODY", "PRIVATE COMMENT BODY", "annotationPosition", "futureData", `"version"`} {
		if strings.Contains(out, forbidden) {
			t.Errorf("stable JSON leaked %q:\n%s", forbidden, out)
		}
	}
}

func TestAnnotationListJSONL(t *testing.T) {
	srv := annotationServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--jsonl", "annotation", "list", "ATTACH01")
	if err != nil {
		t.Fatalf("annotation list JSONL: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSONL lines = %d:\n%s", len(lines), out)
	}
	for i, line := range lines {
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if doc["schema"] != float64(2) || doc["kind"] != "annotation" {
			t.Fatalf("line %d header = %#v", i, doc)
		}
	}
}

func TestAnnotationListRawPreservesEnvelopes(t *testing.T) {
	srv := annotationServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--raw", "annotation", "list", "ATTACH01")
	if err != nil {
		t.Fatalf("annotation list raw: %v", err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode raw output: %v\n%s", err, out)
	}
	fixture := fmt.Sprintf("[%s,%s]", annotationLaterEnvelope, annotationEarlierEnvelope)
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw output changed Zotero envelopes:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAnnotationListEmptyMalformedAndInputErrors(t *testing.T) {
	srv := annotationServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--json", "annotation", "list", "EMPTY001")
	if err != nil {
		t.Fatalf("empty JSON: %v", err)
	}
	if !strings.Contains(out, `"data": []`) || !strings.Contains(out, `"shown": 0`) {
		t.Fatalf("empty JSON = %s", out)
	}
	human, _, err := runCLI(srv.URL, "annotation", "list", "EMPTY001")
	if err != nil {
		t.Fatalf("empty human: %v", err)
	}
	if human != "No annotations for attachment EMPTY001.\n" {
		t.Fatalf("empty human = %q", human)
	}
	raw, _, err := runCLI(srv.URL, "--raw", "annotation", "list", "EMPTY001")
	if err != nil {
		t.Fatalf("empty raw: %v", err)
	}
	if strings.TrimSpace(raw) != "[]" {
		t.Fatalf("empty raw = %q", raw)
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing argument", args: []string{"annotation", "list"}, want: "missing attachment key"},
		{name: "not found", args: []string{"annotation", "list", "MISSING1"}, want: `no attachment with key "MISSING1"`},
		{name: "not attachment", args: []string{"annotation", "list", "PARENT01"}, want: `type "journalArticle", not attachment`},
		{name: "malformed annotation", args: []string{"annotation", "list", "MALFORM1"}, want: "no annotation type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI(srv.URL, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAnnotationListGroupLibrary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/groups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":42,"data":{"id":42,"name":"Research Group"}}]`))
	})
	mux.HandleFunc("GET /api/groups/42/items/GROUPATT", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"key":"GROUPATT","data":{"itemType":"attachment"}}`))
	})
	mux.HandleFunc("GET /api/groups/42/items/GROUPATT/children", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"key":"GROUPANN","data":{"itemType":"annotation","parentItem":"GROUPATT","annotationType":"highlight","annotationSortIndex":"00001"}}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "-L", "Research Group", "--json", "annotation", "list", "GROUPATT")
	if err != nil {
		t.Fatalf("group annotation list: %v", err)
	}
	for _, want := range []string{`"type": "group"`, `"id": 42`, `"attachmentKey": "GROUPATT"`} {
		if !strings.Contains(out, want) {
			t.Errorf("group output missing %q:\n%s", want, out)
		}
	}
}

func TestAnnotationListWebProfile(t *testing.T) {
	t.Setenv("ZOTGO_API_KEY", "test-key")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /keys/current", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key header")
		}
		_, _ = w.Write([]byte(`{"userID":77,"username":"ada","access":{"user":{"library":true}}}`))
	})
	mux.HandleFunc("GET /users/77/items/WEBATT01", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"key":"WEBATT01","data":{"itemType":"attachment"}}`))
	})
	mux.HandleFunc("GET /users/77/items/WEBATT01/children", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key header")
		}
		_, _ = w.Write([]byte(`[{"key":"WEBANN01","data":{"itemType":"annotation","parentItem":"WEBATT01","annotationType":"ink","annotationSortIndex":"00001"}}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--web", "--json", "annotation", "list", "WEBATT01")
	if err != nil {
		t.Fatalf("web annotation list: %v", err)
	}
	if !strings.Contains(out, `"key": "WEBANN01"`) || !strings.Contains(out, `"attachmentKey": "WEBATT01"`) {
		t.Fatalf("web output = %s", out)
	}
}
