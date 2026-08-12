package zotero

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAttachmentUploadFullSequence(t *testing.T) {
	const (
		apiKey    = "persistent-local-key"
		uploadKey = "UPLOADKEY1234567890"
		content   = "%PDF-1.7\nattachment bytes\n"
		md5       = "0123456789abcdef0123456789abcdef"
	)
	serverID := strings.Repeat("S", 12)
	var baseURL string
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		calls = append(calls, "probe")
		w.Header().Set("Zotero-Server-ID", serverID)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /api/users/0/items/ATTACH01/file", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != apiKey || r.Header.Get("Zotero-Server-ID") != serverID {
			t.Errorf("write credentials = key:%q server:%q", r.Header.Get("Zotero-API-Key"), r.Header.Get("Zotero-Server-ID"))
		}
		if r.Header.Get("If-None-Match") != "*" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("write headers = %#v", r.Header)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read form: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if upload := values.Get("upload"); upload != "" {
			calls = append(calls, "register")
			if upload != uploadKey {
				t.Errorf("register upload = %q", upload)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		calls = append(calls, "authorize")
		want := map[string]string{
			"md5": md5, "filename": "image.png", "filesize": "26", "mtime": "1700000000000", "contentType": "image/png",
		}
		for key, value := range want {
			if values.Get(key) != value {
				t.Errorf("form %s = %q, want %q", key, values.Get(key), value)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"url": baseURL + "/api/local/uploads/" + uploadKey, "uploadKey": uploadKey,
			"contentType": "image/png", "prefix": "", "suffix": "",
		})
	})
	mux.HandleFunc("POST /api/local/uploads/"+uploadKey, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "upload")
		if r.Header.Get("Zotero-API-Key") != "" || r.Header.Get("Zotero-Server-ID") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("upload receiver got credentials: %#v", r.Header)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" || r.ContentLength != int64(len(content)) {
			t.Errorf("upload metadata = type:%q length:%d", r.Header.Get("Content-Type"), r.ContentLength)
		}
		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upload: %v", err)
		}
		if string(got) != content {
			t.Errorf("upload bytes = %q", got)
		}
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	client := New(srv.URL)
	client.SetLocalKey(apiKey)
	metadata := AttachmentUploadMetadata{
		MD5: md5, Filename: "image.png", Size: int64(len(content)), MTime: 1700000000000, ContentType: "image/png",
	}
	authorization, err := client.AuthorizeAttachmentUpload(context.Background(), UserLibrary(), "ATTACH01", metadata)
	if err != nil {
		t.Fatalf("AuthorizeAttachmentUpload: %v", err)
	}
	if authorization.Exists || authorization.UploadKey != uploadKey {
		t.Fatalf("authorization = %#v", authorization)
	}
	if err := client.UploadAuthorizedAttachment(context.Background(), authorization, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("UploadAuthorizedAttachment: %v", err)
	}
	if err := client.RegisterAttachmentUpload(context.Background(), UserLibrary(), "ATTACH01", authorization.UploadKey); err != nil {
		t.Fatalf("RegisterAttachmentUpload: %v", err)
	}
	wantCalls := []string{"probe", "authorize", "upload", "register"}
	if strings.Join(calls, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestAttachmentFileFormEncode(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "space", filename: "Example Paper.pdf", want: "Example%20Paper.pdf"},
		{name: "literal plus", filename: "Example+Paper.pdf", want: "Example%2BPaper.pdf"},
		{name: "Unicode", filename: "résumé 文献.pdf", want: "r%C3%A9sum%C3%A9%20%E6%96%87%E7%8C%AE.pdf"},
		{name: "percent and reserved", filename: "100% [final]&notes.pdf", want: "100%25%20%5Bfinal%5D%26notes.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := attachmentFileFormEncode(url.Values{"filename": {tt.filename}})
			if encoded != "filename="+tt.want {
				t.Fatalf("encoded form = %q, want %q", encoded, "filename="+tt.want)
			}
			if strings.Contains(encoded, "+") {
				t.Fatalf("encoded form contains an ambiguous plus: %q", encoded)
			}
		})
	}
}

func TestAuthorizeAttachmentUploadExists(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Zotero-Server-ID", "SERVERID1234")
	})
	mux.HandleFunc("POST /api/users/0/items/ATTACH01/file", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"exists":1}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := New(srv.URL)
	client.SetLocalKey("key")
	authorization, err := client.AuthorizeAttachmentUpload(context.Background(), UserLibrary(), "ATTACH01", AttachmentUploadMetadata{
		MD5: strings.Repeat("a", 32), Filename: "paper.pdf", Size: 1, MTime: 1700000000000,
	})
	if err != nil {
		t.Fatalf("AuthorizeAttachmentUpload: %v", err)
	}
	if !authorization.Exists || authorization.URL != "" || authorization.UploadKey != "" {
		t.Fatalf("authorization = %#v", authorization)
	}
}

func TestAuthorizeAttachmentUploadGroupRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Zotero-Server-ID", "SERVERID1234")
	})
	mux.HandleFunc("POST /api/groups/42/items/GROUPATT/file", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "key" || r.Header.Get("If-None-Match") != "*" {
			t.Errorf("headers = %#v", r.Header)
		}
		_, _ = w.Write([]byte(`{"exists":1}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := New(srv.URL)
	client.SetLocalKey("key")
	authorization, err := client.AuthorizeAttachmentUpload(context.Background(), GroupLibrary(42, "Research"), "GROUPATT", AttachmentUploadMetadata{
		MD5: strings.Repeat("a", 32), Filename: "paper.pdf", Size: 1, MTime: 1700000000000,
	})
	if err != nil || !authorization.Exists {
		t.Fatalf("authorization=%#v err=%v", authorization, err)
	}
}

func TestAttachmentUploadMetadataValidation(t *testing.T) {
	valid := AttachmentUploadMetadata{MD5: strings.Repeat("a", 32), Filename: "paper.pdf", Size: 1, MTime: 1700000000000}
	tests := []struct {
		name   string
		mutate func(*AttachmentUploadMetadata)
		want   string
	}{
		{name: "md5", mutate: func(m *AttachmentUploadMetadata) { m.MD5 = "ABC" }, want: "MD5"},
		{name: "path", mutate: func(m *AttachmentUploadMetadata) { m.Filename = "dir/paper.pdf" }, want: "bare filename"},
		{name: "dot", mutate: func(m *AttachmentUploadMetadata) { m.Filename = ".." }, want: "bare filename"},
		{name: "negative size", mutate: func(m *AttachmentUploadMetadata) { m.Size = -1 }, want: "file size"},
		{name: "large", mutate: func(m *AttachmentUploadMetadata) { m.Size = MaxAttachmentFileSize + 1 }, want: "file size"},
		{name: "negative mtime", mutate: func(m *AttachmentUploadMetadata) { m.MTime = -1 }, want: "modification time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := valid
			tt.mutate(&metadata)
			if err := validateAttachmentUploadMetadata(metadata); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateLocalUploadURL(t *testing.T) {
	client := New("http://127.0.0.1:23119")
	const key = "UPLOADKEY"
	good := "http://127.0.0.1:23119/api/local/uploads/" + key
	if err := client.validateLocalUploadURL(good, key); err != nil {
		t.Fatalf("good URL: %v", err)
	}
	for _, raw := range []string{
		"https://127.0.0.1:23119/api/local/uploads/" + key,
		"http://localhost:23119/api/local/uploads/" + key,
		"http://127.0.0.1:23119/api/local/uploads/OTHER",
		good + "?key=secret",
		"http://user@127.0.0.1:23119/api/local/uploads/" + key,
	} {
		if err := client.validateLocalUploadURL(raw, key); err == nil {
			t.Errorf("unsafe URL accepted: %s", raw)
		}
	}
}

func TestUploadAuthorizedAttachmentDoesNotFollowRedirect(t *testing.T) {
	redirected := false
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/local/uploads/UPLOADKEY", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/receiver")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("GET /receiver", func(w http.ResponseWriter, _ *http.Request) {
		redirected = true
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := New(srv.URL)
	err := client.UploadAuthorizedAttachment(context.Background(), AttachmentUploadAuthorization{
		URL: srv.URL + "/api/local/uploads/UPLOADKEY", UploadKey: "UPLOADKEY",
	}, bytes.NewReader([]byte("x")), 1)
	var statusErr StatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusFound {
		t.Fatalf("error = %v, want StatusError(302)", err)
	}
	if redirected {
		t.Fatal("upload followed redirect")
	}
}

func TestAttachmentFileFormDoesNotFollowRedirect(t *testing.T) {
	redirected := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Zotero-Server-ID", strings.Repeat("S", 12))
	})
	mux.HandleFunc("POST /api/users/0/items/ATTACH01/file", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/capture")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("GET /capture", func(w http.ResponseWriter, r *http.Request) {
		redirected = true
		if r.Header.Get("Zotero-API-Key") != "" {
			t.Error("redirect received Zotero API key")
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := New(srv.URL)
	client.SetLocalKey("key")
	_, err := client.AuthorizeAttachmentUpload(context.Background(), UserLibrary(), "ATTACH01", AttachmentUploadMetadata{
		MD5: strings.Repeat("a", 32), Filename: "paper.pdf", Size: 1, MTime: 1700000000000,
	})
	var statusErr StatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusFound {
		t.Fatalf("error = %v, want StatusError(302)", err)
	}
	if redirected {
		t.Fatal("authenticated form request followed redirect")
	}
}

func TestAttachmentFileFormErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		want   error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: ErrWriteUnauthorized},
		{name: "precondition required", status: http.StatusPreconditionRequired, want: ErrPreconditionRequired},
		{name: "precondition failed", status: http.StatusPreconditionFailed, want: ErrPreconditionFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Zotero-Server-ID", "SERVERID1234")
			})
			mux.HandleFunc("POST /api/users/0/items/ATTACH01/file", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			client := New(srv.URL)
			client.SetLocalKey("key")
			_, err := client.AuthorizeAttachmentUpload(context.Background(), UserLibrary(), "ATTACH01", AttachmentUploadMetadata{
				MD5: strings.Repeat("a", 32), Filename: "paper.pdf", Size: 1, MTime: 1700000000000,
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAuthorizeAttachmentUploadRejectsMalformedResponse(t *testing.T) {
	for _, body := range []string{
		`null`,
		`{"exists":2}`,
		`{"uploadKey":"KEY"}`,
		`{"url":"http://example.invalid/api/local/uploads/KEY","uploadKey":"KEY","prefix":"","suffix":""}`,
		`{"url":"PLACEHOLDER/api/local/uploads/KEY","uploadKey":"KEY","prefix":"unexpected","suffix":""}`,
	} {
		t.Run(body, func(t *testing.T) {
			var baseURL string
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Zotero-Server-ID", "SERVERID1234")
			})
			mux.HandleFunc("POST /api/users/0/items/ATTACH01/file", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(strings.ReplaceAll(body, "PLACEHOLDER", baseURL)))
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			baseURL = srv.URL
			client := New(srv.URL)
			client.SetLocalKey("key")
			_, err := client.AuthorizeAttachmentUpload(context.Background(), UserLibrary(), "ATTACH01", AttachmentUploadMetadata{
				MD5: strings.Repeat("a", 32), Filename: "paper.pdf", Size: 1, MTime: 1700000000000,
			})
			if err == nil {
				t.Fatal("malformed authorization response accepted")
			}
		})
	}
}
