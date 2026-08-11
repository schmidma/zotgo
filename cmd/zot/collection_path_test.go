package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collectionPathServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/collections", func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Total-Results", "4")
		switch r.URL.Query().Get("start") {
		case "":
			w.Header().Set("Link", `<`+baseURL+`/api/users/0/collections?start=2>; rel="next"`)
			_, _ = w.Write([]byte(`[
				{"key":"ROOT0001","version":"","data":{"key":"ROOT0001","name":"Research","parentCollection":false}},
				{"key":"MID00001","version":"","data":{"key":"MID00001","name":"Projects","parentCollection":"ROOT0001"}}
			]`))
		case "2":
			_, _ = w.Write([]byte(`[
				{"key":"LEAF0001","version":"","data":{"key":"LEAF0001","name":"Methods","parentCollection":"MID00001"}},
				{"key":"OTHER001","version":"","data":{"key":"OTHER001","name":"Other","parentCollection":null}}
			]`))
		default:
			t.Errorf("unexpected pagination start %q", r.URL.Query().Get("start"))
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	srv := httptest.NewServer(mux)
	baseURL = srv.URL
	return srv, &requests
}

func TestCollectionPathHelpAndHuman(t *testing.T) {
	srv, requests := collectionPathServer(t)
	defer srv.Close()

	help, _, err := runCLI(srv.URL, "collection", "path", "--help")
	if err != nil {
		t.Fatalf("collection path help: %v", err)
	}
	for _, want := range []string{"<collection-key>...", "root to leaf", "request order", "--raw is unavailable"} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q:\n%s", want, help)
		}
	}

	out, _, err := runCLI(srv.URL, "collection", "path", "LEAF0001", "ROOT0001")
	if err != nil {
		t.Fatalf("collection path: %v", err)
	}
	for _, want := range []string{"KEY", "PATH", "LEAF0001", "Research / Projects / Methods", "ROOT0001", "2 collection paths"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "LEAF0001") > strings.LastIndex(out, "ROOT0001") {
		t.Fatalf("human output changed request order:\n%s", out)
	}
	if *requests != 2 {
		t.Fatalf("collection requests = %d, want two paginated requests", *requests)
	}
}

func TestCollectionPathJSON(t *testing.T) {
	srv, _ := collectionPathServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--json", "collection", "path", "LEAF0001", "ROOT0001")
	if err != nil {
		t.Fatalf("collection path JSON: %v", err)
	}
	var doc struct {
		Schema int    `json:"schema"`
		Kind   string `json:"kind"`
		Data   []struct {
			Key         string `json:"key"`
			Name        string `json:"name"`
			ParentKey   string `json:"parentKey"`
			DisplayPath string `json:"displayPath"`
			Segments    []struct {
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"segments"`
		} `json:"data"`
		Meta struct {
			Shown int `json:"shown"`
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if doc.Schema != 2 || doc.Kind != "collection-paths" || doc.Meta.Shown != 2 || doc.Meta.Total != 2 || len(doc.Data) != 2 {
		t.Fatalf("document = %#v", doc)
	}
	leaf := doc.Data[0]
	if leaf.Key != "LEAF0001" || leaf.Name != "Methods" || leaf.ParentKey != "MID00001" || leaf.DisplayPath != "Research / Projects / Methods" || len(leaf.Segments) != 3 {
		t.Fatalf("leaf = %#v", leaf)
	}
	if leaf.Segments[0].Key != "ROOT0001" || leaf.Segments[2].Key != "LEAF0001" {
		t.Fatalf("segments = %#v", leaf.Segments)
	}
	if doc.Data[1].Key != "ROOT0001" || doc.Data[1].ParentKey != "" || len(doc.Data[1].Segments) != 1 {
		t.Fatalf("root = %#v", doc.Data[1])
	}
	for _, forbidden := range []string{`"version"`, `"meta":{"numCollections"`} {
		if strings.Contains(out, forbidden) {
			t.Errorf("stable JSON leaked %q:\n%s", forbidden, out)
		}
	}
}

func TestCollectionPathJSONLPreservesRequestOrderAndDuplicates(t *testing.T) {
	srv, _ := collectionPathServer(t)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--jsonl", "collection", "path", "ROOT0001", "LEAF0001", "ROOT0001")
	if err != nil {
		t.Fatalf("collection path JSONL: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("JSONL lines = %d, want 3:\n%s", len(lines), out)
	}
	wantKeys := []string{"ROOT0001", "LEAF0001", "ROOT0001"}
	for i, line := range lines {
		var doc struct {
			Schema int    `json:"schema"`
			Kind   string `json:"kind"`
			Data   struct {
				Key string `json:"key"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatalf("decode line %d: %v", i, err)
		}
		if doc.Schema != 2 || doc.Kind != "collection-path" || doc.Data.Key != wantKeys[i] {
			t.Errorf("line %d = %#v, want key %s", i, doc, wantKeys[i])
		}
	}
}

func TestCollectionPathRejectsRawAndBadArgumentsBeforeNetwork(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "raw", args: []string{"--raw", "collection", "path", "ROOT0001"}, want: "derived from multiple collection records"},
		{name: "missing", args: []string{"collection", "path"}, want: "missing collection key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI("http://127.0.0.1:1", tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}

	args := []string{"collection", "path"}
	for i := 0; i < maxCollectionPathKeys+1; i++ {
		args = append(args, fmt.Sprintf("KEY%05d", i))
	}
	_, _, err := runCLI("http://127.0.0.1:1", args...)
	if err == nil || !strings.Contains(err.Error(), "maximum is 100") {
		t.Fatalf("too-many error = %v", err)
	}
}

func TestCollectionPathMissingCollection(t *testing.T) {
	srv, _ := collectionPathServer(t)
	defer srv.Close()

	_, _, err := runCLI(srv.URL, "collection", "path", "LOST0001")
	if err == nil || !strings.Contains(err.Error(), `resolve collection paths: no collection with key "LOST0001"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCollectionPathGroupLibrary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/groups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":42,"data":{"id":42,"name":"Research Group"}}]`))
	})
	mux.HandleFunc("GET /api/groups/42/collections", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"key":"GROUPRT1","version":"","data":{"key":"GROUPRT1","name":"Group Root","parentCollection":false}},
			{"key":"GROUPLF1","version":"","data":{"key":"GROUPLF1","name":"Group Leaf","parentCollection":"GROUPRT1"}}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "-L", "Research Group", "--json", "collection", "path", "GROUPLF1")
	if err != nil {
		t.Fatalf("group collection path: %v", err)
	}
	for _, want := range []string{`"type": "group"`, `"id": 42`, `"displayPath": "Group Root / Group Leaf"`} {
		if !strings.Contains(out, want) {
			t.Errorf("group output missing %q:\n%s", want, out)
		}
	}
}

func TestCollectionPathWebProfile(t *testing.T) {
	t.Setenv("ZOTGO_API_KEY", "test-key")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /keys/current", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key")
		}
		_, _ = w.Write([]byte(`{"userID":77,"username":"ada","access":{"user":{"library":true}}}`))
	})
	mux.HandleFunc("GET /users/77/collections", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing Web API key")
		}
		_, _ = w.Write([]byte(`[
			{"key":"WEBROOT1","version":9,"data":{"key":"WEBROOT1","name":"Web Root","parentCollection":false}},
			{"key":"WEBLEAF1","version":10,"data":{"key":"WEBLEAF1","name":"Web Leaf","parentCollection":"WEBROOT1"}}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--web", "--json", "collection", "path", "WEBLEAF1")
	if err != nil {
		t.Fatalf("web collection path: %v", err)
	}
	if !strings.Contains(out, `"key": "WEBLEAF1"`) || !strings.Contains(out, `"displayPath": "Web Root / Web Leaf"`) {
		t.Fatalf("web output = %s", out)
	}
}
