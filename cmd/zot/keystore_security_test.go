package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLocalKeyReplacesPermissiveFileSecurely(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ZOTGO_CONFIG_DIR", dir)
	path := filepath.Join(dir, "local-api-key")
	if err := os.WriteFile(path, []byte("old-key\n"), 0o666); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o666); err != nil {
			t.Fatalf("chmod seed: %v", err)
		}
	}
	if err := saveLocalKey("new-key"); err != nil {
		t.Fatalf("saveLocalKey: %v", err)
	}
	if got := loadLocalKey(); got != "new-key" {
		t.Fatalf("loadLocalKey = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat key: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("key mode = %o, want 600", info.Mode().Perm())
		}
		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat config dir: %v", err)
		}
		if dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("config mode = %o, want 700", dirInfo.Mode().Perm())
		}
	}
}

func TestLocalKeySymlinkIsRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows runners")
	}
	dir := t.TempDir()
	t.Setenv("ZOTGO_CONFIG_DIR", dir)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("untouched\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	path := filepath.Join(dir, "local-api-key")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if got := loadLocalKey(); got != "" {
		t.Fatalf("loaded key through symlink: %q", got)
	}
	if err := saveLocalKey("new-key"); err == nil {
		t.Fatal("saveLocalKey followed symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "untouched\n" {
		t.Fatalf("symlink target changed: %q", data)
	}
}

func TestSaveLocalKeyRejectsMultilineValue(t *testing.T) {
	t.Setenv("ZOTGO_CONFIG_DIR", t.TempDir())
	for _, key := range []string{"", "one\ntwo", "one\rtwo"} {
		if err := saveLocalKey(key); err == nil {
			t.Errorf("invalid key accepted: %q", key)
		}
	}
}
