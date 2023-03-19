
# notion-ical

Convert a Notion database to iCal events.

Supports Notion API access and Notion exports.

## Usage

For now, building from source is required.

```sh
go install github.com/serverwentdown/notion-ical/cmd/notion-ical@latest
notion-ical --help
```

Example:

```sh
notion-ical \
  --api-key secret_... \
  --database-id xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  save --output Calendar_Name.ical
```

<!-- vim: set conceallevel=2 et ts=2 sw=2: -->
