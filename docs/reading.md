# Reading your library

All read commands work against either endpoint (local by default, or the Web API
under `--web` — see [profiles](profiles.md)).

```bash
zot list                       # top-level items (default 25)
zot list -c "Smart Grid" -n 50 # items in a collection, by name or key
zot list --tag ml --tag review # items with all the given tags
zot search "state estimation"  # search by title/creator/year
zot search algae --everything  # include full text and notes
zot show HRAC4E44              # one item with its attachments and notes
zot relation list HRAC4E44     # outgoing relation predicates and targets
zot annotation list ABCD1234   # compact annotations under one attachment
zot collections               # collections as a tree (--flat for a list)
zot stats                     # library-wide counts
```

Global flags: `--library`/`-L` selects a group library (by name or id; default is
My Library), and `--url` overrides the endpoint address.

`relation list` preserves each complete target URI. Its stable machine output
also provides `targetKey` when the URI identifies another Zotero item, so scripts
can inspect that item without parsing the URI.

## Annotations

`zot annotation list ATTACHMENT_KEY` reads the direct annotations under one
attachment and orders them by Zotero's document sort index. Human and stable
machine output show the annotation key, type, page label, color, sort index, and
whether text or a comment is present. They deliberately omit annotation text,
comment bodies, and document position data; use `--raw` when those Zotero-owned
fields are essential.

The command accepts an attachment key, not the key of its parent bibliographic
item. Its count appears in the human footer and JSON `meta`, so a separate count
request is unnecessary.

## Export

`export` hands off to Zotero's own translators, so no bibliography formatting is
reimplemented here:

```bash
zot export bibtex -c Polyhedra   # BibTeX (from Zotero), scoped to a collection
zot export csljson -o refs.json  # -o writes to a file (atomically) instead of stdout
zot export ris                   # ris, biblatex, csv, mods, tei, rdf_* …
zot export summary-md            # zotgo's own summary shapes: json, summary-csv, summary-md
```

The Zotero translators are `bibtex`, `biblatex`, `csljson`, `csv`, `mods`, `ris`,
`tei`, and the `rdf_*` variants. zotgo shapes only `json`, `summary-csv`, and
`summary-md` itself.

`mods`, `tei`, and `rdf_*` wrap each page of results in a single XML root element,
so zotgo exports them only when the result fits in one page rather than emitting a
document with two roots; narrow the query with `-c`/`-t` if you hit that.

For scripting the output of any read command, see
[machine-readable output](machine-output.md).
