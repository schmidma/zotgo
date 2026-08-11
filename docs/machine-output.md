# Machine-readable output

Every command speaks three mutually exclusive machine formats.

```sh
zot --json list       # one versioned document
zot --jsonl list      # one self-describing document per line
zot --raw list        # Zotero's own response, untouched
```

## `--json`

`--json` wraps stable zotgo DTOs in a versioned envelope. The shape is the same
for every command, so a script learns it once:

```json
{
  "schema": 2,
  "kind": "items",
  "library": { "type": "user", "id": 0, "name": "My Library" },
  "data": [ { "key": "AAAA1111", "type": "journalArticle", "title": "Algae paper" } ],
  "meta": { "shown": 25, "total": 312 }
}
```

`kind` says what `data` holds: `items`, `item`, `collections`, `collection`,
`collection-paths`, `collection-path`, `stats`, or `health`. A `health` document carries `endpoint` and `capabilities`,
so a script can check for `write` support rather than assume it. `schema` is
bumped only when a field changes meaning or disappears — new fields may appear at
any time, so ignore the ones you don't know.

### Collection paths

`zot --json collection path KEY...` emits a `collection-paths` array in request
order. Each record has the requested collection's `key`, `name`, and
`parentKey`, plus root-to-leaf `segments` containing both keys and names.
`displayPath` joins the names with ` / ` for display only; scripts should use
`segments` rather than parse that string. JSONL uses the singular
`collection-path` kind for each self-describing record.

The command accepts 1–100 keys and reads the paginated collection index once.
Missing collections or parents, malformed parent references, and cycles are
errors. `--raw` is unavailable because each path is derived from multiple
collection records rather than one Zotero response.

### No `version` field

Items, collections, and collection-path records carry **no `version`**. A Zotero object version belongs to
the endpoint that issued it, and the Local API's has no meaning zotgo can promise:
it is the *server* version, so it does not move when you edit an item locally
without syncing, and the local write API replaces it with an unrelated local
counter. Sending one to the Web API as a write precondition is a data-integrity
hazard. If you need Zotero's number anyway, take it from `--raw`, which is
explicitly outside this contract.

## `--jsonl`

`--jsonl` emits one document per line, each repeating `schema`, `kind`, and
`library`. Every line therefore stands alone, and a stream survives being
truncated, split, or concatenated with another:

```sh
zot --jsonl list | jq -r '.data | "\(.key)\t\(.title)"'
```

## `--raw`

`--raw` passes Zotero's API response straight through. It is an escape hatch for
fields zotgo does not model, and it is **not covered by `schema`**: its shape is
Zotero's and changes when Zotero changes. `stats`, `doctor`, and `collection
path` reject `--raw`, because zotgo derives them and there is no single
underlying Zotero response.
