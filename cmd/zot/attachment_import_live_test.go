//go:build live

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestLiveAttachmentImportRoundTrip(t *testing.T) {
	key := loadLocalKey()
	if key == "" {
		t.Skip("no remembered Local API key; authorize a write with Always Allow first")
	}
	client := zotero.New(os.Getenv("ZOTGO_BASE_URL"))
	client.SetLocalKey(key)
	health := client.CheckHealth(context.Background())
	if !health.Supports(zotero.CapabilityManagedFileUpload) {
		t.Skip("running Zotero does not report managed-file upload support")
	}
	ctx := context.Background()
	library := zotero.UserLibrary()
	parentResult, err := client.CreateItems(ctx, library, []json.RawMessage{
		json.RawMessage(`{"itemType":"journalArticle","title":"zotgo live attachment import parent"}`),
	})
	if errors.Is(err, zotero.ErrWriteUnauthorized) {
		t.Skip("remembered Local API key was revoked; reauthorize with Always Allow before running live import")
	}
	if err != nil || !parentResult.Ok() {
		t.Fatalf("create parent: result=%+v err=%v", parentResult, err)
	}
	parentKey := parentResult.Successful["0"].Key
	var attachmentKey string
	defer func() {
		keys := []string{parentKey}
		if attachmentKey != "" {
			keys = append([]string{attachmentKey}, keys...)
		}
		version, err := client.LibraryVersion(context.Background(), library)
		if err != nil {
			t.Errorf("cleanup library version: %v", err)
			return
		}
		if err := client.DeleteItems(context.Background(), library, keys, version); err != nil {
			t.Errorf("cleanup items %v: %v", keys, err)
		}
	}()

	source := filepath.Join(t.TempDir(), "live-paper.pdf")
	contents := []byte("%PDF-1.7\n% zotgo live attachment import\n%%EOF\n")
	if err := os.WriteFile(source, contents, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mtime := time.UnixMilli(1700000000000)
	if err := os.Chtimes(source, mtime, mtime); err != nil {
		t.Fatalf("set source mtime: %v", err)
	}

	out, _, runErr := runCLI(client.BaseURL(),
		"--json", "attachment", "import", "--parent", parentKey,
		"--file", source, "--filename", "live-paper.pdf", "--title", "Live PDF",
		"--source-url", "https://example.com/zotgo-live-paper.pdf", "--yes",
	)
	var doc struct {
		Kind string `json:"kind"`
		Data struct {
			Status        string  `json:"status"`
			AttachmentKey *string `json:"attachmentKey"`
			Verification  struct {
				Parent, ManagedStorage, Title, SourceURL bool
				Filename, ContentType, Size, Checksum    bool
			} `json:"verification"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode command output: %v\n%s", err, out)
	}
	if doc.Data.AttachmentKey != nil {
		attachmentKey = *doc.Data.AttachmentKey
	}
	if runErr != nil {
		t.Fatalf("attachment import: %v\n%s", runErr, out)
	}
	if doc.Kind != "attachment-import" || doc.Data.Status != "imported" || attachmentKey == "" {
		t.Fatalf("document = %+v", doc)
	}
	v := doc.Data.Verification
	if !v.Parent || !v.ManagedStorage || !v.Title || !v.SourceURL || !v.Filename || !v.ContentType || !v.Size || !v.Checksum {
		t.Fatalf("verification = %+v", v)
	}

	raw, err := client.RawItem(ctx, library, attachmentKey)
	if err != nil {
		t.Fatalf("read imported attachment: %v", err)
	}
	var item struct {
		Data struct {
			ParentItem string `json:"parentItem"`
			LinkMode   string `json:"linkMode"`
			Filename   string `json:"filename"`
			MD5        string `json:"md5"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatalf("decode imported attachment: %v", err)
	}
	if item.Data.ParentItem != parentKey || item.Data.LinkMode != "imported_file" || item.Data.Filename != "live-paper.pdf" || item.Data.MD5 == "" {
		t.Fatalf("imported attachment data = %+v", item.Data)
	}
}
