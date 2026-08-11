package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// configDir is zotgo's configuration directory: $ZOTGO_CONFIG_DIR when set,
// otherwise the platform user-config dir with a zotgo/ subdirectory.
func configDir() (string, error) {
	if d := os.Getenv("ZOTGO_CONFIG_DIR"); d != "" {
		return d, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "zotgo"), nil
}

// localKeyPath is where zotgo persists an approved local API key. It is a
// local-only write credential (it authorizes writes to the Zotero on this
// machine), kept owner-readable only.
func localKeyPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local-api-key"), nil
}

// loadLocalKey returns the persisted local API key, or "" if none is stored or
// it cannot be read.
func loadLocalKey() string {
	path, err := localKeyPath()
	if err != nil {
		return ""
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) || file.Chmod(0o600) != nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// saveLocalKey atomically replaces the key with owner-only permissions.
func saveLocalKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return errors.New("invalid empty or multiline local API key")
	}
	path, err := localKeyPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := secureConfigDir(dir); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("local API key path is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(dir, ".local-api-key.tmp-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tmp)
	}()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := io.WriteString(file, key+"\n"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func secureConfigDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("zotgo config path is not a regular directory")
	}
	return os.Chmod(dir, 0o700)
}

// clearLocalKey removes the persisted key. A missing file is not an error.
func clearLocalKey() error {
	path, err := localKeyPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
