# Writing

`zot item`, `zot collection`, and `zot tag` create and modify data on the
**local** endpoint. Writes need a Zotero build with the local write API
([zotero/zotero#5015](https://github.com/zotero/zotero/pull/5015)); older builds
are read-only, and `zot doctor` reports which you have under the `write`
capability. Writes are refused on `--web`.

## Items

```bash
zot item template book                 # print a blank skeleton for an item type
zot item template book > b.json        # …fill it in, then:
zot item create --file b.json          # create (also reads JSON on stdin)
zot item patch KEY < patch.json        # partial update; fields you omit are untouched
zot item delete KEY1 KEY2              # destructive; lists what it will remove first
```

`create` accepts a single item object or an array. `patch` takes a JSON object of
just the fields to change.

## Collections

```bash
zot collection create "Smart Grid" -p PARENTKEY   # -p/--parent is optional
zot collection rename KEY "New Name"
zot collection delete KEY                          # the collection's items are kept
```

## Tags

```bash
zot tag add urgent todo --item ITEMKEY   # add tags to one item
zot tag remove todo --item ITEMKEY       # remove tags from one item
zot tag delete urgent                    # remove a tag from EVERY item (library-wide)
```

`tag add`/`remove` edit one item's tags and preserve the rest; `tag delete`
strips a tag from the whole library.

## Managed attachments

```bash
zot attachment import \
  --parent ITEMKEY \
  --file figure.png \
  --title "Figure 1" \
  --source-url https://example.org/figure.png
```

`attachment import` attaches one local file to an existing bibliographic parent.
It creates `imported_file` metadata, uploads the bytes to Zotero, registers them
as a Zotero-managed file, and then verifies the parent, title, source URL,
filename, media type, MD5, and byte length. The media type is detected from the
staged file's first 512 bytes; unrecognized formats use
`application/octet-stream`. `--content-type` overrides detection when the user
knows a more precise MIME type. File extensions do not determine the media type.
`--filename` overrides the local basename. `--source-url` is provenance stored
on the attachment; zotgo does not download it. Imports are capped at 128 MiB
while Zotero's current Local API receiver buffers each upload in memory before
staging it to disk.

Before writing, zotgo checks the parent's direct attachments. An existing exact
MD5 is a successful no-op unless `--allow-duplicate` is set. This check is
best-effort rather than atomic: Zotero exposes no create-unless-this-parent-has-
no-matching-checksum precondition.

The operation is multi-stage. If metadata creation succeeds but a later upload
phase fails, zotgo reports the attachment key and last completed stage and does
not attempt an unsafe rollback. `--raw` is unavailable because no single Zotero
response represents the whole operation.

`item create` can still create attachment metadata, but it does not ingest local
bytes. It rejects local `path` and premature `filename` fields for new
`imported_file` items and directs users to `attachment import`.

## Safety and authorization

Every write **surfaces the target library, shows what it will do, and asks to
confirm**:

- `--dry-run` previews without writing (and without authorizing).
- `--yes` skips the confirmation prompt for scripts.

The first write prompts for approval in Zotero. Choosing **Always Allow** stores a
local API key in `~/.config/zotgo/local-api-key` (mode `0600`) so later writes
don't re-prompt; **Allow** grants a single-use key. Managed attachment import
requires **Always Allow**, because metadata creation, upload authorization, and
registration are separate authenticated writes. Set `ZOTGO_CONFIG_DIR` to
relocate the config directory.

Writes carry Zotero's required `Zotero-Server-ID` and `If-Unmodified-Since-Version`
preconditions, so if the library changed since the command read it, the write is
rejected and reported rather than silently overwriting.
