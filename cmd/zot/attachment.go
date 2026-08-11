package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/CameronBrooks11/zotgo/internal/output"
	"github.com/CameronBrooks11/zotgo/internal/render"
	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func attachmentCommand() *cli.Command {
	return &cli.Command{
		Name:  "attachment",
		Usage: "inspect attachment metadata",
		Commands: []*cli.Command{
			attachmentShowCommand(),
		},
	}
}

func attachmentShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "show attachment details and conservative file status",
		ArgsUsage: "<attachment-key>",
		Description: "Reports metadata Zotero advertised without opening its database or downloading the file. " +
			"File status does not claim portable filesystem existence; use --raw for the complete Zotero envelope.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			key := cmd.Args().First()
			if key == "" {
				return errors.New("missing attachment key (usage: zot attachment show <attachment-key>)")
			}
			c, lib, err := resolveLibrary(ctx, cmd)
			if err != nil {
				return err
			}
			mode, err := outputMode(cmd)
			if err != nil {
				return err
			}
			raw, err := c.RawItem(ctx, lib, key)
			if err != nil {
				return attachmentReadError(err, key, lib.Name)
			}
			envelope, err := attachmentEnvelope(raw)
			if err != nil {
				return fmt.Errorf("decode attachment %q: %w", key, err)
			}
			if envelope.Key != key {
				return fmt.Errorf("decode attachment %q: response has key %q", key, envelope.Key)
			}
			attachment, err := envelope.Attachment()
			if err != nil {
				return fmt.Errorf("decode attachment %q: %w", key, err)
			}
			w := out(cmd)
			if mode == output.ModeHuman {
				render.Attachment(w, attachment)
				return nil
			}
			return emitOne(w, mode, output.KindAttachment, output.NewLibrary(lib), output.NewAttachment(attachment), raw)
		},
	}
}

func attachmentEnvelope(raw json.RawMessage) (zotero.Envelope, error) {
	var item struct {
		Key   string                 `json:"key"`
		Links map[string]zotero.Link `json:"links"`
		Data  json.RawMessage        `json:"data"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return zotero.Envelope{}, err
	}
	if item.Key == "" {
		return zotero.Envelope{}, errors.New("missing attachment key")
	}
	return zotero.Envelope{Key: item.Key, Links: item.Links, Data: item.Data}, nil
}

func attachmentReadError(err error, key, library string) error {
	if errors.Is(err, zotero.ErrNotFound) {
		return fmt.Errorf("no attachment with key %q in %s", key, library)
	}
	return friendly(err)
}
