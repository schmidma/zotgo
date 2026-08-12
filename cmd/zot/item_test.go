package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestParseItemsInput(t *testing.T) {
	single, err := parseItemsInput([]byte(`{"itemType":"book","title":"One"}`))
	if err != nil || len(single) != 1 {
		t.Fatalf("single object → %d items, err %v", len(single), err)
	}
	arr, err := parseItemsInput([]byte(`[{"itemType":"book"},{"itemType":"note"}]`))
	if err != nil || len(arr) != 2 {
		t.Fatalf("array → %d items, err %v", len(arr), err)
	}
	if _, err := parseItemsInput([]byte("   ")); err == nil {
		t.Error("empty input should error")
	}
	if _, err := parseItemsInput([]byte("{bad json")); err == nil {
		t.Error("invalid JSON should error")
	}
}

func TestConfirm(t *testing.T) {
	cases := map[string]bool{"y\n": true, "yes\n": true, "Y\n": true, "n\n": false, "\n": false, "nope\n": false}
	for in, want := range cases {
		if got := confirm(strings.NewReader(in), io.Discard, "ok?"); got != want {
			t.Errorf("confirm(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSortedIndices(t *testing.T) {
	m := map[string]string{"2": "b", "0": "a", "10": "c", "1": "x"}
	got := sortedIndices(m)
	want := []string{"0", "1", "2", "10"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sortedIndices = %v, want %v", got, want)
	}
}

func TestKeystore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ZOTGO_CONFIG_DIR", dir)

	if k := loadLocalKey(); k != "" {
		t.Fatalf("fresh config has a key: %q", k)
	}
	if err := saveLocalKey("SECRET123"); err != nil {
		t.Fatalf("saveLocalKey: %v", err)
	}
	if k := loadLocalKey(); k != "SECRET123" {
		t.Errorf("loadLocalKey = %q, want SECRET123", k)
	}
	// Stored owner-only (Unix permission model only).
	path := filepath.Join(dir, "local-api-key")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("key file mode = %o, want 600", perm)
		}
	}
	if err := clearLocalKey(); err != nil {
		t.Fatalf("clearLocalKey: %v", err)
	}
	if k := loadLocalKey(); k != "" {
		t.Errorf("key survived clear: %q", k)
	}
	// Clearing again is not an error.
	if err := clearLocalKey(); err != nil {
		t.Errorf("clear on missing file errored: %v", err)
	}
}

func TestReportWriteResult_OrdersByIndex(t *testing.T) {
	res := zotero.WriteResult{
		Successful: map[string]zotero.Envelope{
			"0": mustEnvelope(`{"key":"AAA"}`),
			"2": mustEnvelope(`{"key":"CCC"}`),
		},
		Failed: map[string]zotero.WriteFailure{
			"1": {Key: "BBB", Code: 400, Message: "bad"},
		},
	}
	var b strings.Builder
	reportWriteResult(&b, res)
	out := b.String()
	if !strings.Contains(out, "created AAA") || !strings.Contains(out, "created CCC") || !strings.Contains(out, "failed [1] BBB") {
		t.Errorf("report missing lines:\n%s", out)
	}
}

func mustEnvelope(s string) zotero.Envelope {
	var e zotero.Envelope
	_ = json.Unmarshal([]byte(s), &e)
	return e
}

func TestParsePatchInput(t *testing.T) {
	if _, err := parsePatchInput([]byte(`{"title":"New"}`)); err != nil {
		t.Errorf("valid object → %v", err)
	}
	for name, in := range map[string]string{
		"array":   `[{"title":"x"}]`,
		"empty":   `   `,
		"emptyOb": `{}`,
		"bad":     `{nope`,
	} {
		if _, err := parsePatchInput([]byte(in)); err == nil {
			t.Errorf("%s should error", name)
		}
	}
}

func TestValidateItemPatchSafety(t *testing.T) {
	tests := []struct {
		name    string
		item    string
		patch   string
		wantErr bool
	}{
		{name: "managed file filename", item: `{"key":"ATTACH01","data":{"itemType":"attachment","linkMode":"imported_file","filename":"old.pdf","tags":[],"md5":null,"mtime":null}}`, patch: `{"filename":"new.pdf"}`, wantErr: true},
		{name: "managed URL filename", item: `{"key":"ATTACH01","data":{"itemType":"attachment","linkMode":"imported_url","filename":"old.pdf","tags":[],"md5":null,"mtime":null}}`, patch: `{"filename":"new.pdf"}`, wantErr: true},
		{name: "managed title", item: `{"key":"ATTACH01","data":{"itemType":"attachment","linkMode":"imported_file","filename":"old.pdf","tags":[],"md5":null,"mtime":null}}`, patch: `{"title":"New title"}`},
		{name: "linked file filename", item: `{"key":"ATTACH01","data":{"itemType":"attachment","linkMode":"linked_file","filename":"old.pdf","tags":[],"md5":null,"mtime":null}}`, patch: `{"filename":"new.pdf"}`},
		{name: "bibliographic filename", item: `{"key":"ITEM0001","data":{"itemType":"journalArticle","title":"Paper"}}`, patch: `{"filename":"ignored.pdf"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var item zotero.Envelope
			if err := json.Unmarshal([]byte(tt.item), &item); err != nil {
				t.Fatalf("decode item: %v", err)
			}
			err := validateItemPatchSafety(item, json.RawMessage(tt.patch))
			if tt.wantErr && (err == nil || !strings.Contains(err.Error(), "without renaming the stored file")) {
				t.Fatalf("error = %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestItemPatchRejectsManagedFilenameBeforeWrite(t *testing.T) {
	t.Setenv("ZOTGO_CONFIG_DIR", t.TempDir())
	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/ATTACH01", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"key":"ATTACH01","version":7,"data":{"itemType":"attachment","linkMode":"imported_file","filename":"old.pdf","tags":[],"md5":null,"mtime":null}}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	patchFile := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(patchFile, []byte(`{"filename":"new.pdf"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCLI(srv.URL, "item", "patch", "ATTACH01", "--file", patchFile, "--yes")
	if err == nil || !strings.Contains(err.Error(), "without renaming the stored file") {
		t.Fatalf("error = %v", err)
	}
	if stdout != "" || strings.Join(requests, ",") != "GET /api/users/0/items/ATTACH01" {
		t.Fatalf("stdout = %q, requests = %v", stdout, requests)
	}
}

func TestPatchFields_Sorted(t *testing.T) {
	got := patchFields(json.RawMessage(`{"title":"a","abstractNote":"b","date":"c"}`))
	if strings.Join(got, ",") != "abstractNote,date,title" {
		t.Errorf("patchFields = %v, want sorted", got)
	}
}
