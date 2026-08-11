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

const maxCollectionPathKeys = 100

func collectionPathCommand() *cli.Command {
	return &cli.Command{
		Name:      "path",
		Usage:     "resolve collection ancestry from root to leaf",
		ArgsUsage: "<collection-key>...",
		Description: "Resolves one or more collection keys in request order. " +
			"Paths are derived from collection records, so --raw is unavailable.",
		Action: collectionPathAction,
	}
}

func collectionPathAction(ctx context.Context, cmd *cli.Command) error {
	keys := cmd.Args().Slice()
	if len(keys) == 0 {
		return errors.New("missing collection key (usage: zot collection path <collection-key>...)")
	}
	if len(keys) > maxCollectionPathKeys {
		return fmt.Errorf("too many collection keys: got %d, maximum is %d", len(keys), maxCollectionPathKeys)
	}
	mode, err := outputMode(cmd)
	if err != nil {
		return err
	}
	if mode == output.ModeRaw {
		return fmt.Errorf("%w: collection paths are derived from multiple collection records", output.ErrRawUnavailable)
	}
	c, lib, err := resolveLibrary(ctx, cmd)
	if err != nil {
		return err
	}
	collections, err := c.AllRawCollections(ctx, lib, zotero.CollectionsOptions{})
	if err != nil {
		return friendly(err)
	}
	paths, err := zotero.ResolveRawCollectionPaths(collections, keys)
	if err != nil {
		return fmt.Errorf("resolve collection paths: %w", err)
	}
	w := out(cmd)
	if mode == output.ModeHuman {
		render.CollectionPaths(w, paths)
		return nil
	}
	records := output.NewCollectionPaths(paths)
	return emitSet(w, mode, output.KindCollectionPaths, output.KindCollectionPath,
		output.NewLibrary(lib), records, len(records), len(records), nil)
}
