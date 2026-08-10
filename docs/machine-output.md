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

`kind` says what `data` holds: `items`, `item`, `notes`, `note`, `collections`,
`collection`, `stats`, or `health`. A `health` document carries `endpoint` and
`capabilities`,
so a script can check for `write` support rather than assume it. `schema` is
bumped only when a field changes meaning or disappears — new fields may appear at
any time, so ignore the ones you don't know.

### Notes

`zot --json note list ITEM_KEY` emits a modified-descending `notes` array. Each
record carries its key, parent key, added/modified dates, tags, and `hasContent`.
The `html` field is absent, so listing notes cannot leak their bodies.

`zot --json note get NOTE_KEY` emits one `note` record with the same fields and
an additional `html` field containing Zotero's exact rich-note HTML. `html` is
present even for an empty note. JSONL uses the singular `note` kind for both
forms.

For `note list`, `--raw` fetches all pages and joins their complete Zotero item
envelopes into one array without reshaping fields. For `note get`, it emits the
single complete envelope. Raw shape and server order remain outside the stable
contract.

### No `version` field

Items, collections, and stable note records carry **no `version`**. A Zotero object version belongs to
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
Zotero's and changes when Zotero changes. `stats` and `doctor` reject `--raw`,
because zotgo derives them and there is no underlying Zotero response.
