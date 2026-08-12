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

`kind` says what `data` holds: `items`, `item`, `attachment`,
`attachment-import`, `collections`, `collection`, `stats`, or `health`. A `health` document carries `endpoint` and
`capabilities`,
so a script can check for `write` support rather than assume it. `schema` is
bumped only when a field changes meaning or disappears — new fields may appear at
any time, so ignore the ones you don't know.

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

### Attachment imports

After argument and preflight validation succeeds, `zot --json attachment import
...` emits one `attachment-import` record. JSONL emits the same self-describing
record on one line. Argument, source-file, target-library, parent, and capability
validation errors are regular command errors and emit no record. `status` is
`planned`, `duplicate`, `imported`, `partial`, or `failed`; `stage` identifies
the last completed phase. The record always contains parent key, filename, media type,
size, and MD5. The media type is detected from staged bytes unless an explicit
`--content-type` overrides it. Attachment key, file status, focused verification,
and structured failure are explicit nullable fields.

Successful verification covers the parent relationship, managed storage, title,
source URL, filename, media type, byte length, and checksum. A partial
result retains the newly created attachment key for diagnosis and deliberately
does not claim rollback. Local source paths, API credentials, upload URLs and
keys, and Zotero versions never appear. `--raw` is unavailable because the
result composes several requests.

### No `version` field

Items, collections, and stable attachment records carry **no `version`**. A Zotero object version belongs to
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
Zotero's and changes when Zotero changes. `stats`, `doctor`, and `attachment
import` reject `--raw`, because zotgo derives them and there is no single
underlying Zotero response.
