package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/CameronBrooks11/zotgo/internal/output"
	"github.com/CameronBrooks11/zotgo/internal/render"
	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func annotationCommand() *cli.Command {
	return &cli.Command{
		Name:        "annotation",
		Usage:       "inspect attachment annotations",
		Description: "List compact annotation metadata for one attachment with `zot annotation list`; stable output omits annotation bodies.",
		Commands: []*cli.Command{
			annotationListCommand(),
		},
	}
}

func annotationListCommand() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "list annotations belonging to an attachment",
		ArgsUsage: "<attachment-key>",
		Description: "Stable output contains compact metadata in document order, without annotation text, " +
			"comments, or document position data. Use --raw for Zotero's complete annotation envelopes.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			attachmentKey := cmd.Args().First()
			if attachmentKey == "" {
				return errors.New("missing attachment key; see `zot annotation list --help`")
			}
			c, lib, err := resolveLibrary(ctx, cmd)
			if err != nil {
				return err
			}
			attachment, err := c.Item(ctx, lib, attachmentKey)
			if err != nil {
				return annotationReadError(err, attachmentKey, lib.Name)
			}
			if itemType := attachment.ItemType(); itemType != "attachment" {
				return fmt.Errorf("item %q in %s has type %q, not attachment", attachmentKey, lib.Name, itemType)
			}

			mode, err := outputMode(cmd)
			if err != nil {
				return err
			}
			opts := zotero.ChildrenOptions{ItemType: "annotation", Limit: 100}
			w := out(cmd)
			if mode == output.ModeRaw {
				raw, err := c.AllRawChildItems(ctx, lib, attachmentKey, opts)
				if err != nil {
					return friendly(err)
				}
				return emitSet(w, mode, output.KindAnnotations, output.KindAnnotation,
					output.NewLibrary(lib), []output.Annotation(nil), len(raw), len(raw), raw)
			}

			children, err := c.AllChildItems(ctx, lib, attachmentKey, opts)
			if err != nil {
				return friendly(err)
			}
			annotations, err := zotero.Annotations(children)
			if err != nil {
				return fmt.Errorf("decode annotations for attachment %q: %w", attachmentKey, err)
			}
			if mode == output.ModeHuman {
				render.Annotations(w, attachmentKey, annotations)
				return nil
			}
			records := output.NewAnnotations(annotations)
			return emitSet(w, mode, output.KindAnnotations, output.KindAnnotation,
				output.NewLibrary(lib), records, len(records), len(records), children)
		},
	}
}

func annotationReadError(err error, key, library string) error {
	if errors.Is(err, zotero.ErrNotFound) {
		return fmt.Errorf("no attachment with key %q in %s", key, library)
	}
	return friendly(err)
}
