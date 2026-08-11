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

func noteCommand() *cli.Command {
	return &cli.Command{
		Name:        "note",
		Usage:       "inspect Zotero notes",
		Description: "Use `note list` for body-free child-note metadata and `note get` when exact rich HTML is explicitly needed.",
		Commands: []*cli.Command{
			noteListCommand(),
			noteGetCommand(),
		},
	}
}

func noteListCommand() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "list direct child notes without their bodies",
		ArgsUsage: "<item-key>",
		Description: "Lists compact note metadata in modified-descending order. " +
			"Use note get for one note's HTML or --raw for complete Zotero envelopes.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			itemKey := cmd.Args().First()
			if itemKey == "" {
				return errors.New("missing item key; see `zot note list --help`")
			}
			c, lib, err := resolveLibrary(ctx, cmd)
			if err != nil {
				return err
			}
			mode, err := outputMode(cmd)
			if err != nil {
				return err
			}
			opts := zotero.ChildrenOptions{ItemType: "note", Limit: 100}
			w := out(cmd)
			rawParent, err := c.RawItem(ctx, lib, itemKey)
			if err != nil {
				return noteParentReadError(err, itemKey, lib.Name)
			}
			itemType, err := rawItemType(rawParent, itemKey)
			if err != nil {
				return err
			}
			if err := validateNoteParentType(itemType, itemKey, lib.Name); err != nil {
				return err
			}
			raw, err := c.AllRawChildItems(ctx, lib, itemKey, opts)
			if err != nil {
				return friendly(err)
			}
			if mode == output.ModeRaw {
				return emitSet(w, mode, output.KindNotes, output.KindNote,
					output.NewLibrary(lib), []output.Note(nil), len(raw), len(raw), raw)
			}

			children, err := noteEnvelopes(raw)
			if err != nil {
				return fmt.Errorf("decode notes for item %q: %w", itemKey, err)
			}
			notes, err := zotero.Notes(children)
			if err != nil {
				return fmt.Errorf("decode notes for item %q: %w", itemKey, err)
			}
			for _, note := range notes {
				if note.ParentKey != itemKey {
					return fmt.Errorf("note %q belongs to %q, not %q", note.Key, note.ParentKey, itemKey)
				}
			}
			if mode == output.ModeHuman {
				render.Notes(w, itemKey, notes)
				return nil
			}
			records := output.NewNotes(notes)
			return emitSet(w, mode, output.KindNotes, output.KindNote,
				output.NewLibrary(lib), records, len(records), len(records), children)
		},
	}
}

func noteGetCommand() *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "get one note including its rich HTML",
		ArgsUsage: "<note-key>",
		Description: "Returns Zotero's rich-note HTML exactly in the stable html field. " +
			"Use --raw for the complete Zotero item envelope.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			noteKey := cmd.Args().First()
			if noteKey == "" {
				return errors.New("missing note key; see `zot note get --help`")
			}
			c, lib, err := resolveLibrary(ctx, cmd)
			if err != nil {
				return err
			}
			mode, err := outputMode(cmd)
			if err != nil {
				return err
			}
			w := out(cmd)
			raw, err := c.RawItem(ctx, lib, noteKey)
			if err != nil {
				return noteReadError(err, noteKey, lib.Name)
			}
			if err := validateRawNote(raw, noteKey); err != nil {
				return err
			}
			if mode == output.ModeRaw {
				return emitOne(w, mode, output.KindNote, output.NewLibrary(lib), output.Note{}, raw)
			}

			item, err := noteEnvelope(raw)
			if err != nil {
				return fmt.Errorf("decode note %q: %w", noteKey, err)
			}
			note, err := item.Note()
			if err != nil {
				return fmt.Errorf("decode note %q: %w", noteKey, err)
			}
			if mode == output.ModeHuman {
				render.Note(w, note)
				return nil
			}
			return emitOne(w, mode, output.KindNote, output.NewLibrary(lib), output.NewNote(note, true), raw)
		},
	}
}

func validateNoteParentType(itemType, key, library string) error {
	switch itemType {
	case "attachment", "annotation", "note", "":
		return fmt.Errorf("item %q in %s has type %q, not a bibliographic item", key, library, itemType)
	default:
		return nil
	}
}

func validateRawNote(raw json.RawMessage, key string) error {
	itemType, err := rawItemType(raw, key)
	if err != nil {
		return err
	}
	if itemType != "note" {
		return fmt.Errorf("item %q has type %q, not note", key, itemType)
	}
	return nil
}

func noteEnvelopes(rawItems []json.RawMessage) ([]zotero.Envelope, error) {
	envelopes := make([]zotero.Envelope, 0, len(rawItems))
	for _, raw := range rawItems {
		envelope, err := noteEnvelope(raw)
		if err != nil {
			return nil, err
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes, nil
}

func noteEnvelope(raw json.RawMessage) (zotero.Envelope, error) {
	var item struct {
		Key  string          `json:"key"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return zotero.Envelope{}, err
	}
	if item.Key == "" {
		return zotero.Envelope{}, errors.New("missing note key")
	}
	return zotero.Envelope{Key: item.Key, Data: item.Data}, nil
}

func rawItemType(raw json.RawMessage, key string) (string, error) {
	var item struct {
		Key  string `json:"key"`
		Data struct {
			ItemType string `json:"itemType"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return "", fmt.Errorf("decode item %q: %w", key, err)
	}
	if item.Key == "" {
		return "", fmt.Errorf("decode item %q: missing item key", key)
	}
	if item.Key != key {
		return "", fmt.Errorf("decode item %q: response has key %q", key, item.Key)
	}
	return item.Data.ItemType, nil
}

func noteParentReadError(err error, key, library string) error {
	if errors.Is(err, zotero.ErrNotFound) {
		return fmt.Errorf("no item with key %q in %s", key, library)
	}
	return friendly(err)
}

func noteReadError(err error, key, library string) error {
	if errors.Is(err, zotero.ErrNotFound) {
		return fmt.Errorf("no note with key %q in %s", key, library)
	}
	return friendly(err)
}
