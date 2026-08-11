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

func relationCommand() *cli.Command {
	return &cli.Command{
		Name:        "relation",
		Usage:       "inspect item relations",
		Description: "List one item's outgoing relation predicates and targets with `zot relation list`.",
		Commands: []*cli.Command{
			relationListCommand(),
		},
	}
}

func relationListCommand() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "list one item's outgoing relations",
		ArgsUsage: "<item-key>",
		Description: "Relations retain Zotero's predicate and complete target URI. " +
			"Machine output also extracts targetKey when the URI identifies a Zotero item.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			key := cmd.Args().First()
			if key == "" {
				return errors.New("missing item key; see `zot relation list --help`")
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
			if mode == output.ModeRaw {
				raw, err := c.RawItem(ctx, lib, key)
				if err != nil {
					return relationReadError(err, key, lib.Name)
				}
				return emitSet(w, mode, output.KindRelations, output.KindRelation,
					output.NewLibrary(lib), []output.Relation(nil), 0, 0, raw)
			}

			item, err := c.Item(ctx, lib, key)
			if err != nil {
				return relationReadError(err, key, lib.Name)
			}
			if mode == output.ModeHuman {
				relations, err := item.Relations()
				if err != nil {
					return fmt.Errorf("decode relations for item %q: %w", key, err)
				}
				render.Relations(w, item.Key, relations)
				return nil
			}
			records, err := output.NewRelations(item)
			if err != nil {
				return fmt.Errorf("shape relations for item %q: %w", key, err)
			}
			return emitSet(w, mode, output.KindRelations, output.KindRelation,
				output.NewLibrary(lib), records, len(records), len(records), item)
		},
	}
}

func relationReadError(err error, key, library string) error {
	if errors.Is(err, zotero.ErrNotFound) {
		return fmt.Errorf("no item with key %q in %s", key, library)
	}
	return friendly(err)
}
