package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/CameronBrooks11/zotgo/internal/output"
	"github.com/CameronBrooks11/zotgo/internal/render"
	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func showCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "show one item and its children",
		ArgsUsage: "<item-key>",
		Description: "Stable --json and --jsonl put the shaped item at .data and its shaped children at .data.children. " +
			"Raw output losslessly composes the complete Zotero envelopes as .item and .children; inspect fields with `zot --raw show KEY | jq '.item.data'`.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			key := cmd.Args().First()
			if key == "" {
				return errors.New("missing item key (usage: zot show <item-key>)")
			}
			c, lib, err := resolveLibrary(ctx, cmd)
			if err != nil {
				return err
			}
			mode, err := outputMode(cmd)
			if err != nil {
				return err
			}
			if mode == output.ModeRaw {
				item, err := c.RawItem(ctx, lib, key)
				if err != nil {
					if errors.Is(err, zotero.ErrNotFound) {
						return fmt.Errorf("no item with key %q in %s", key, lib.Name)
					}
					return friendly(err)
				}
				children, err := c.RawItemChildren(ctx, lib, key)
				if err != nil {
					return friendly(err)
				}
				return writeRawShow(out(cmd), item, children)
			}

			item, err := c.Item(ctx, lib, key)
			if err != nil {
				if errors.Is(err, zotero.ErrNotFound) {
					return fmt.Errorf("no item with key %q in %s", key, lib.Name)
				}
				return friendly(err)
			}
			children, _, err := c.ItemChildren(ctx, lib, key)
			if err != nil {
				return friendly(err)
			}

			w := out(cmd)
			if mode != output.ModeHuman {
				return emitOne(w, mode, output.KindItem, output.NewLibrary(lib),
					output.NewItemWithChildren(item, children), nil)
			}
			render.Item(w, item, children)
			return nil
		},
	}
}

func writeRawShow(w io.Writer, item, children []byte) error {
	for _, part := range [][]byte{[]byte("{\"item\":"), item, []byte(",\"children\":"), children, []byte("}\n")} {
		if _, err := w.Write(part); err != nil {
			return err
		}
	}
	return nil
}
