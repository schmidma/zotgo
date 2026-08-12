package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/urfave/cli/v3"

	"github.com/CameronBrooks11/zotgo/internal/output"
	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

type stagedAttachment struct {
	file        *os.File
	filename    string
	contentType string
	size        int64
	mtime       int64
	md5         string
}

func (s *stagedAttachment) close() {
	name := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(name)
}

func attachmentImportCommand() *cli.Command {
	return &cli.Command{
		Name:  "import",
		Usage: "attach a local file as a Zotero-managed file",
		Description: "Creates an imported_file child, uploads the file through Zotero's Local API, and verifies managed metadata. " +
			"The MIME type is detected from the staged bytes unless --content-type overrides it. --source-url stores provenance and is not downloaded. " +
			"Managed imports are local-only, capped at 128 MiB, and require 'Always Allow' authorization.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "parent", Usage: "existing bibliographic parent item key"},
			&cli.StringFlag{Name: "file", Usage: "local file path"},
			&cli.StringFlag{Name: "title", Value: "Attachment", Usage: "attachment title"},
			&cli.StringFlag{Name: "source-url", Usage: "source/provenance URL stored on the attachment (not downloaded)"},
			&cli.StringFlag{Name: "filename", Usage: "managed filename (default: local basename)"},
			&cli.StringFlag{Name: "content-type", Usage: "MIME type override (default: detect from file bytes)"},
			&cli.BoolFlag{Name: "allow-duplicate", Usage: "create another attachment even when this parent has the same MD5"},
			&cli.BoolFlag{Name: "dry-run", Usage: "validate and show every planned phase without authorizing or writing"},
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "skip confirmation"},
		},
		Action: attachmentImportAction,
	}
}

func attachmentImportAction(ctx context.Context, cmd *cli.Command) error {
	mode, err := outputMode(cmd)
	if err != nil {
		return err
	}
	if mode == output.ModeRaw {
		return fmt.Errorf("%w: attachment import is a derived multi-stage operation", output.ErrRawUnavailable)
	}
	if mode != output.ModeHuman && !cmd.Bool("dry-run") && !cmd.Bool("yes") {
		return errors.New("attachment import with --json or --jsonl requires --yes (or use --dry-run)")
	}
	if cmd.Bool("web") {
		return errors.New("managed attachment import is local-only; the --web profile is read-only")
	}
	parentKey := strings.TrimSpace(cmd.String("parent"))
	if parentKey == "" {
		return errors.New("missing --parent item key; see `zot attachment import --help`")
	}
	sourcePath := cmd.String("file")
	if sourcePath == "" {
		return errors.New("missing --file path; see `zot attachment import --help`")
	}
	title := strings.TrimSpace(cmd.String("title"))
	if title == "" {
		return errors.New("attachment title must not be empty")
	}
	sourceURL := strings.TrimSpace(cmd.String("source-url"))
	if err := validateAttachmentSourceURL(sourceURL); err != nil {
		return err
	}
	staged, err := stageAttachmentFile(sourcePath, cmd.String("filename"), cmd.String("content-type"))
	if err != nil {
		return err
	}
	defer staged.close()

	client, library, err := resolveLibrary(ctx, cmd)
	if err != nil {
		return err
	}
	if key := loadLocalKey(); key != "" {
		client.SetLocalKey(key)
	}
	if err := validateAttachmentParent(ctx, client, library, parentKey); err != nil {
		return err
	}
	if client.ServerID() == "" {
		return errors.New("this Zotero build has no Local API managed-file upload; update Zotero once a supported release ships")
	}
	duplicate, err := findDuplicateAttachment(ctx, client, library, parentKey, staged.md5, staged.size)
	if err != nil {
		return err
	}

	record := output.AttachmentImport{
		Status: "planned", Stage: "preflight", ParentKey: parentKey,
		Filename: staged.filename, ContentType: staged.contentType,
		Size: staged.size, MD5: staged.md5,
	}
	if duplicate != nil && !cmd.Bool("allow-duplicate") {
		record.Status = "duplicate"
		record.AttachmentKey = &duplicate.Key
		record.Filename = duplicate.Filename
		record.FileStatus = attachmentImportFileStatus(*duplicate)
		return emitAttachmentImport(cmd, mode, output.NewLibrary(library), record)
	}
	if cmd.Bool("dry-run") {
		return emitAttachmentImport(cmd, mode, output.NewLibrary(library), record)
	}

	if mode == output.ModeHuman {
		fmt.Fprintf(out(cmd), "Target: %s — %s\n", library.Name, client.BaseURL())
		fmt.Fprintf(out(cmd), "Parent: %s\nFile: %s (%d bytes)\nMD5: %s\n",
			parentKey, staged.filename, staged.size, staged.md5)
		fmt.Fprintln(out(cmd), "Plan: create metadata → authorize upload → upload bytes → register → verify")
		if duplicate != nil {
			fmt.Fprintf(out(cmd), "Warning: attachment %s has the same MD5; --allow-duplicate was set.\n", duplicate.Key)
		}
		if !cmd.Bool("yes") && !confirm(os.Stdin, out(cmd), fmt.Sprintf("Import %s under %s?", staged.filename, parentKey)) {
			fmt.Fprintln(out(cmd), "Aborted.")
			return nil
		}
	}

	if err := ensureRememberedLocalKey(ctx, cmd, client); err != nil {
		record.Status = "failed"
		return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
			"authorization-required", "managed import requires remembered Zotero authorization")
	}

	attachmentKey, err := createImportedAttachmentMetadata(ctx, client, library, parentKey, title, sourceURL, staged.contentType)
	if err != nil {
		record.Status = "failed"
		return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
			"metadata-create-failed", safeImportFailure("Zotero did not create the attachment metadata", err))
	}
	record.AttachmentKey = &attachmentKey
	record.Status = "partial"
	record.Stage = "metadata-created"

	metadata := zotero.AttachmentUploadMetadata{
		MD5: staged.md5, Filename: staged.filename, Size: staged.size,
		MTime: staged.mtime, ContentType: staged.contentType,
	}
	authorization, err := client.AuthorizeAttachmentUpload(ctx, library, attachmentKey, metadata)
	if err != nil {
		return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
			"upload-authorize-failed", safeImportFailure("Zotero did not authorize the file upload", err))
	}
	record.Stage = "authorized"

	if !authorization.Exists {
		if _, err := staged.file.Seek(0, io.SeekStart); err != nil {
			return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
				"staged-file-failed", "could not rewind the staged attachment")
		}
		if err := client.UploadAuthorizedAttachment(ctx, authorization, staged.file, staged.size); err != nil {
			return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
				"upload-failed", safeImportFailure("Zotero did not accept the file bytes", err))
		}
		record.Stage = "uploaded"
		registerErr := client.RegisterAttachmentUpload(ctx, library, attachmentKey, authorization.UploadKey)
		if registerErr == nil {
			record.Stage = "registered"
		}
		attachment, verification, verifyErr := verifyImportedAttachment(
			ctx, client, library, attachmentKey, parentKey, title, sourceURL, *staged)
		if attachment.Key != "" {
			record.FileStatus = attachmentImportFileStatus(attachment)
			record.Verification = &verification
		}
		if verification.OK() {
			record.Status = "imported"
			record.Stage = "verified"
			record.Filename = attachment.Filename
			if registerErr != nil {
				fmt.Fprintln(errOut(cmd), "zot: warning: registration response failed, but focused verification confirmed the import")
			}
			return emitAttachmentImport(cmd, mode, output.NewLibrary(library), record)
		}
		if registerErr != nil {
			return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
				"register-failed", safeImportFailure("Zotero did not confirm file registration", registerErr))
		}
		return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
			"verification-failed", safeImportFailure("the registered attachment failed focused verification", verifyErr))
	}

	attachment, verification, verifyErr := verifyImportedAttachment(
		ctx, client, library, attachmentKey, parentKey, title, sourceURL, *staged)
	if attachment.Key != "" {
		record.FileStatus = attachmentImportFileStatus(attachment)
		record.Verification = &verification
	}
	if !verification.OK() {
		return finishAttachmentImportFailure(cmd, mode, output.NewLibrary(library), record,
			"verification-failed", safeImportFailure("Zotero reported existing bytes but verification failed", verifyErr))
	}
	record.Status = "imported"
	record.Stage = "verified"
	record.Filename = attachment.Filename
	return emitAttachmentImport(cmd, mode, output.NewLibrary(library), record)
}

func stageAttachmentFile(sourcePath, filenameOverride, contentTypeOverride string) (*stagedAttachment, error) {
	contentTypeOverride, err := validateAttachmentContentType(contentTypeOverride)
	if err != nil {
		return nil, err
	}
	pathInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, errors.New("attachment source is unavailable")
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, errors.New("attachment source must be a regular file")
	}
	source, err := openAttachmentSource(sourcePath)
	if err != nil {
		return nil, errors.New("attachment source could not be opened")
	}
	defer source.Close()
	before, err := source.Stat()
	if err != nil {
		return nil, errors.New("attachment source could not be inspected")
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("attachment source must be a regular file")
	}
	if before.Size() == 0 {
		return nil, errors.New("attachment source is empty")
	}
	if before.Size() > zotero.MaxAttachmentFileSize {
		return nil, fmt.Errorf("attachment source exceeds zotgo's %d-byte managed-upload safety limit", zotero.MaxAttachmentFileSize)
	}
	filename, err := attachmentImportFilename(sourcePath, filenameOverride)
	if err != nil {
		return nil, err
	}

	staged, err := os.CreateTemp("", "zotgo-attachment-import-*")
	if err != nil {
		return nil, fmt.Errorf("create private attachment staging file: %w", err)
	}
	cleanup := func() {
		name := staged.Name()
		_ = staged.Close()
		_ = os.Remove(name)
	}
	hash := md5.New()
	size, err := io.Copy(io.MultiWriter(staged, hash), io.LimitReader(source, zotero.MaxAttachmentFileSize+1))
	if err != nil {
		cleanup()
		return nil, errors.New("attachment source could not be staged")
	}
	after, err := source.Stat()
	if err != nil {
		cleanup()
		return nil, errors.New("attachment source could not be reinspected")
	}
	if size != before.Size() || size > zotero.MaxAttachmentFileSize || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		cleanup()
		return nil, errors.New("attachment source changed while it was being staged")
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, fmt.Errorf("rewind staged attachment: %w", err)
	}
	header := make([]byte, 512)
	n, err := staged.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		cleanup()
		return nil, fmt.Errorf("inspect staged attachment: %w", err)
	}
	contentType := http.DetectContentType(header[:n])
	if contentTypeOverride != "" {
		contentType = contentTypeOverride
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, fmt.Errorf("rewind staged attachment: %w", err)
	}
	mtime := before.ModTime().UnixMilli()
	if mtime < 0 {
		cleanup()
		return nil, errors.New("attachment source modification time predates 1970")
	}
	return &stagedAttachment{
		file: staged, filename: filename, contentType: contentType,
		size: size, mtime: mtime, md5: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func validateAttachmentContentType(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return "", fmt.Errorf("invalid --content-type MIME type: %w", err)
	}
	typeName, subtype, ok := strings.Cut(mediaType, "/")
	if !ok || typeName == "" || subtype == "" || strings.Contains(typeName, "*") || strings.Contains(subtype, "*") {
		return "", errors.New("invalid --content-type MIME type: expected a concrete type/subtype")
	}
	return mime.FormatMediaType(mediaType, params), nil
}

func attachmentImportFilename(sourcePath, override string) (string, error) {
	filename := override
	if filename == "" {
		filename = filepath.Base(filepath.Clean(sourcePath))
	}
	if err := validateManagedFilename(filename); err != nil {
		return "", err
	}
	return filename, nil
}

func validateManagedFilename(filename string) error {
	if filename == "" || filename == "." || filename == ".." || strings.ContainsAny(filename, `/\\`) {
		return errors.New("managed filename must be a bare filename")
	}
	if !utf8.ValidString(filename) || len([]byte(filename)) > 255 {
		return errors.New("managed filename must be valid UTF-8 and no more than 255 bytes")
	}
	if strings.HasSuffix(filename, " ") || strings.HasSuffix(filename, ".") {
		return errors.New("managed filename must not end with a space or dot")
	}
	for _, r := range filename {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) {
			return fmt.Errorf("managed filename contains unsupported character %q", r)
		}
	}
	base := filename
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	reserved := map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"CLOCK$": true, "CONIN$": true, "CONOUT$": true,
		"COM¹": true, "COM²": true, "COM³": true,
		"LPT¹": true, "LPT²": true, "LPT³": true,
	}
	upper := strings.ToUpper(base)
	if reserved[upper] || (len(upper) == 4 &&
		((strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9')) {
		return fmt.Errorf("managed filename %q is reserved on Windows", filename)
	}
	return nil
}

func validateAttachmentSourceURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("--source-url must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return errors.New("--source-url must not contain credentials")
	}
	return nil
}

func validateAttachmentParent(ctx context.Context, client *zotero.Client, library zotero.LibraryRef, parentKey string) error {
	raw, err := client.RawItem(ctx, library, parentKey)
	if err != nil {
		if errors.Is(err, zotero.ErrNotFound) {
			return fmt.Errorf("no parent item with key %q in %s", parentKey, library.Name)
		}
		return friendly(err)
	}
	var parent struct {
		Key  string `json:"key"`
		Data struct {
			ItemType string `json:"itemType"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parent); err != nil {
		return fmt.Errorf("decode parent item %q: %w", parentKey, err)
	}
	if parent.Key != parentKey {
		return fmt.Errorf("decode parent item %q: response has key %q", parentKey, parent.Key)
	}
	if parent.Data.ItemType == "" {
		return fmt.Errorf("decode parent item %q: missing item type", parentKey)
	}
	switch parent.Data.ItemType {
	case "attachment", "note", "annotation":
		return fmt.Errorf("item %q has type %q and cannot be a bibliographic attachment parent", parentKey, parent.Data.ItemType)
	}
	return nil
}

func findDuplicateAttachment(ctx context.Context, client *zotero.Client, library zotero.LibraryRef, parentKey, checksum string, size int64) (*zotero.Attachment, error) {
	rawChildren, err := client.AllRawChildItems(ctx, library, parentKey, zotero.ChildrenOptions{ItemType: "attachment", Limit: 100})
	if err != nil {
		return nil, friendly(err)
	}
	matches := make([]zotero.Attachment, 0)
	for i, raw := range rawChildren {
		envelope, err := attachmentEnvelope(raw)
		if err != nil {
			return nil, fmt.Errorf("decode child attachment %d: %w", i, err)
		}
		attachment, err := envelope.Attachment()
		if err != nil {
			return nil, err
		}
		if attachment.ParentKey != parentKey {
			return nil, fmt.Errorf("attachment %q reports parent %q, expected %q", attachment.Key, attachment.ParentKey, parentKey)
		}
		if attachment.MD5 != nil && strings.EqualFold(*attachment.MD5, checksum) {
			if attachment.Enclosure != nil && attachment.Enclosure.Length != nil && *attachment.Enclosure.Length != size {
				return nil, fmt.Errorf("attachment %q has matching MD5 but conflicting size metadata", attachment.Key)
			}
			matches = append(matches, attachment)
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Key < matches[j].Key })
	return &matches[0], nil
}

func ensureRememberedLocalKey(ctx context.Context, cmd *cli.Command, client *zotero.Client) error {
	if client.HasLocalKey() {
		return nil
	}
	fmt.Fprintln(errOut(cmd), "Authorizing with Zotero — choose 'Always Allow' in the app…")
	remember, err := client.Authorize(ctx, "zotgo")
	if err != nil {
		return writeFriendly(err)
	}
	if !remember {
		return errors.New("attachment import requires 'Always Allow' because it performs multiple authenticated writes")
	}
	if err := saveLocalKey(client.LocalKey()); err != nil {
		fmt.Fprintf(errOut(cmd), "zot: warning: could not save the remembered local key: %v\n", err)
	}
	return nil
}

func createImportedAttachmentMetadata(ctx context.Context, client *zotero.Client, library zotero.LibraryRef, parentKey, title, sourceURL, contentType string) (string, error) {
	item := map[string]any{
		"itemType": "attachment", "parentItem": parentKey,
		"linkMode": "imported_file", "title": title, "contentType": contentType,
	}
	if sourceURL != "" {
		item["url"] = sourceURL
	}
	body, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode attachment metadata: %w", err)
	}
	result, err := client.CreateItemsReturningKeys(ctx, library, []json.RawMessage{body})
	if err != nil {
		return "", writeFriendly(err)
	}
	if len(result.Failed) != 0 {
		failure := result.Failed["0"]
		if failure.Message == "" {
			return "", errors.New("zotero rejected attachment metadata")
		}
		return "", fmt.Errorf("zotero rejected attachment metadata: %s", failure.Message)
	}
	createdKey, ok := result.Successful["0"]
	if !ok || createdKey == "" || len(result.Successful) != 1 || len(result.Unchanged) != 0 {
		return "", errors.New("unexpected attachment create response")
	}
	return createdKey, nil
}

func verifyImportedAttachment(ctx context.Context, client *zotero.Client, library zotero.LibraryRef, key, parentKey, title, sourceURL string, staged stagedAttachment) (zotero.Attachment, output.AttachmentImportVerification, error) {
	raw, err := client.RawItem(ctx, library, key)
	if err != nil {
		return zotero.Attachment{}, output.AttachmentImportVerification{}, friendly(err)
	}
	envelope, err := attachmentEnvelope(raw)
	if err != nil {
		return zotero.Attachment{}, output.AttachmentImportVerification{}, fmt.Errorf("decode imported attachment envelope: %w", err)
	}
	attachment, err := envelope.Attachment()
	if err != nil {
		return zotero.Attachment{}, output.AttachmentImportVerification{}, fmt.Errorf("decode imported attachment: %w", err)
	}
	verification := output.AttachmentImportVerification{
		Parent:         attachment.ParentKey == parentKey,
		ManagedStorage: attachment.LinkMode == "imported_file" && attachment.Enclosure != nil,
		Title:          attachment.Title == title,
		SourceURL:      attachment.URL == sourceURL,
		Filename:       attachment.Filename == staged.filename,
		ContentType:    attachment.ContentType == staged.contentType,
		Size:           attachment.Enclosure != nil && attachment.Enclosure.Length != nil && *attachment.Enclosure.Length == staged.size,
		Checksum:       attachment.MD5 != nil && strings.EqualFold(*attachment.MD5, staged.md5),
	}
	if verification.OK() {
		return attachment, verification, nil
	}
	var mismatches []string
	checks := []struct {
		name string
		ok   bool
	}{
		{"parent", verification.Parent}, {"managed storage", verification.ManagedStorage},
		{"title", verification.Title}, {"source URL", verification.SourceURL},
		{"filename", verification.Filename}, {"content type", verification.ContentType},
		{"size", verification.Size}, {"checksum", verification.Checksum},
	}
	for _, check := range checks {
		if !check.ok {
			mismatches = append(mismatches, check.name)
		}
	}
	return attachment, verification, fmt.Errorf("verification mismatch: %s", strings.Join(mismatches, ", "))
}

func attachmentImportFileStatus(attachment zotero.Attachment) *output.AttachmentFileStatus {
	status := attachment.FileStatus()
	return &output.AttachmentFileStatus{State: status.State, Reason: status.Reason}
}

func safeImportFailure(prefix string, err error) string {
	var statusErr zotero.StatusError
	if errors.As(err, &statusErr) {
		return fmt.Sprintf("%s (HTTP %d)", prefix, statusErr.StatusCode)
	}
	switch {
	case errors.Is(err, zotero.ErrWriteUnauthorized):
		return prefix + " (authorization was rejected or expired)"
	case errors.Is(err, zotero.ErrPreconditionFailed):
		return prefix + " (the attachment changed concurrently)"
	case errors.Is(err, zotero.ErrPreconditionRequired):
		return prefix + " (Zotero required a missing precondition)"
	default:
		return prefix
	}
}

func finishAttachmentImportFailure(cmd *cli.Command, mode output.Mode, library *output.Library, record output.AttachmentImport, code, message string) error {
	record.Failure = &output.AttachmentImportFailure{Code: code, Message: message}
	if err := emitAttachmentImport(cmd, mode, library, record); err != nil {
		return err
	}
	return cli.Exit("", 1)
}

func emitAttachmentImport(cmd *cli.Command, mode output.Mode, library *output.Library, record output.AttachmentImport) error {
	if mode == output.ModeHuman {
		w := out(cmd)
		fmt.Fprintf(w, "Status: %s\nStage: %s\nParent: %s\n", record.Status, record.Stage, record.ParentKey)
		if record.AttachmentKey != nil {
			fmt.Fprintf(w, "Attachment: %s\n", *record.AttachmentKey)
		}
		fmt.Fprintf(w, "Filename: %s\nSize: %d\nMD5: %s\n", record.Filename, record.Size, record.MD5)
		if record.FileStatus != nil {
			fmt.Fprintf(w, "File status: %s — %s\n", record.FileStatus.State, record.FileStatus.Reason)
		}
		if record.Status == "planned" {
			fmt.Fprintln(w, "Dry run — metadata alone would not contain file bytes; upload and registration are required.")
		}
		if record.Failure != nil {
			fmt.Fprintf(w, "Failure: %s\n", record.Failure.Message)
		}
		return nil
	}
	return emitOne(out(cmd), mode, output.KindAttachmentImport, library, record, nil)
}
