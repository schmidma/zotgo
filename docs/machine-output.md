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

`kind` says what `data` holds: `items`, `item`, `attachment`, `relations`,
`relation`, `annotations`, `annotation`, `notes`, `note`, `collections`,
`collection`, `collection-paths`, `collection-path`, `stats`, `health`, or
`item-mutations`. A `health` document carries `endpoint` and `capabilities`, so a script can check for `write` support rather than assume
it. `schema` is bumped only when a field changes meaning or disappears — new
fields may appear at any time, so ignore the ones you don't know.

### Item writes

`zot item create`, `patch`, and `delete` emit an `item-mutations` document whose
`data` is always an array, including a single create or patch. Every record has
the request `index`, its `operation`, and a status such as `planned`, `created`,
`unchanged`, `failed`, `patched`, `deleted`, or `notFound`. Context fields (`key`,
`type`, `title`, and sorted patch `fields`) appear when known; failed creates
carry a structured `failure` with `code` and `message`. Dry runs use `planned`
and perform no authorization or write.

Non-dry-run item writes require `--yes` with `--json` or `--jsonl`. This prevents
an automation command from falling back to an interactive prompt. A partial
create emits all per-item outcomes in request order, then exits with status 1.
Mutation documents have no pagination `meta` and never expose Zotero object
versions or the raw request body.

### Annotations

`zot --json annotation list ATTACHMENT_KEY` emits an ordered `annotations`
array. Each record carries the annotation and attachment keys, annotation type,
page label, color, Zotero sort index, and `hasText`/`hasComment` flags. Text,
comment bodies, and document position data are intentionally absent. JSONL uses
the singular `annotation` kind for each self-describing record.

`--raw` fetches all result pages and joins their complete Zotero annotation item
envelopes into one array. Envelope fields are not reshaped, but both their shape
and server order remain outside the stable contract.

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

### Attachments

`zot --json attachment show ATTACHMENT_KEY` emits one `attachment` record. It
contains stable attachment metadata, nullable `md5`/`mtime`, a nullable
`enclosure`, and `fileStatus` with a state and reason. JSONL emits the same record
on one self-describing line.

The status describes only API evidence:

- `metadata-available` — Zotero advertised a location and size metadata.
- `location-advertised` — Zotero advertised a location without size metadata.
- `linked-unverified` — the linked file cannot be checked portably through the API.
- `unavailable` — an imported attachment has no advertised location.
- `not-applicable` — the attachment links to a URL rather than a managed file.
- `unknown` — zotgo does not recognize the link mode.

None of these states is a durable filesystem-existence assertion. `--raw`
preserves the complete single-item envelope.

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

Items, collections, collection-path records, stable attachment records, stable
annotation records, and stable note records carry **no `version`**. A Zotero
object version belongs to the endpoint that issued it,
and the Local API's has no meaning zotgo can promise:
it is the *server* version, so it does not move when you edit an item locally
without syncing, and the local write API replaces it with an unrelated local
counter. Sending one to the Web API as a write precondition is a data-integrity
hazard. If you need Zotero's number anyway, take it from `--raw`, which is
explicitly outside this contract.

### Relations

`zot --json relation list ITEM_KEY` emits an ordered `relations` array. Each
record carries the source `itemKey`, Zotero predicate, and complete target URI;
`targetKey` is present when the URI identifies another Zotero item. JSONL uses
the singular `relation` kind for each self-describing record.

## `--jsonl`

`--jsonl` emits one document per line, each repeating `schema`, `kind`, and
`library`. Every line therefore stands alone, and a stream survives being
truncated, split, or concatenated with another. Item write records use the
singular kind `item-mutation`:

```sh
zot --jsonl list | jq -r '.data | "\(.key)\t\(.title)"'
```

## `--raw`

`--raw` passes Zotero's API response straight through. It is an escape hatch for
fields zotgo does not model, and it is **not covered by `schema`**: its shape is
Zotero's and changes when Zotero changes. `stats`, `doctor`, `collection path`,
and the three item write commands reject `--raw`, because their output is
derived and is not a raw Zotero response.
