# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`subs` is a small Go HTTP service that serves per-UUID VPN subscription pages.
A user requests `http://host/<uuid>`; the service looks up `work_dir/<uuid>/`,
parses the `subs.yaml` peer list found there, and renders it as an HTML page.

## Commands

```sh
make build                         # build for the local system -> .bin/subs
make build-linux                   # build for linux/amd64       -> .bin/subs-linux-amd64
make build-all                     # both
make test                          # go test ./...
make vet                           # go vet ./...
make clean                         # remove .bin/

go run . [path/to/config.yaml]     # run directly (config arg is optional)
go run . -h | --help               # show usage
go run . -v | --version            # show version (main.go `version` var)
```

The server listens on the config's `port` (default `defaultPort` = 9876, in `main.go`;
resolved by `Config.listenAddr`). Build output goes to `.bin/`. `-h`/`-v` are
handled in `main` (via `usage()`) before any config lookup and exit immediately.

## Configuration

Config is YAML. Search precedence (`locateConfig` in config.go): explicit CLI
argument → `./.subs.yaml` → `~/.subs.yaml` → `/etc/subs.yaml`.

```yaml
host: 127.0.0.1        # optional, defaults to 127.0.0.1 (bind behind a reverse proxy)
port: 9876             # optional, defaults to 9876
work_dir: .subs        # optional, defaults to ".subs"; relative paths resolve against the config file's directory. Must exist at startup, otherwise the process exits with an error.
title: Hello World!    # used as the HTML <title> and the page heading
description: ...        # optional; shown as a tooltip when hovering the heading
headers:               # added to every response
  title: hello world!
api_enabled: false     # optional, defaults to false
api_host: 127.0.0.1    # optional, defaults to 127.0.0.1
api_port: 4321         # optional, defaults to 4321
api_token: change-me   # required when api_enabled is true
max_upload_mb: 10      # optional, defaults to 10
```

Listen addresses are built by `Config.listenAddr`/`apiListenAddr` (host defaults
to `defaultHost` = 127.0.0.1). The upload cap is `Config.maxUploadBytes`
(`max_upload_mb`, default `defaultMaxUploadMB` = 10).

## Management API (api.go)

Enabled by `api_enabled`; runs on `api_port` (default `defaultAPIPort` = 4321) in
a goroutine alongside the page server (`main.go`). Started only when enabled, and
`main` exits if the token is empty. All requests need
`Authorization: Bearer <api_token>` (`apiServer.authorized`, constant-time compare).

All responses share a `success` envelope: success is `{"success":true, ...}`
(`writeSuccess`), any failure is just `{"success":false}` with an appropriate
status code (`writeFail`). Both go through `writeJSON`.

- `POST /api/create` (`handleCreate`) — creates an empty subscription: makes
  `work_dir/<subid>/` with an empty `subs.yaml`. Takes no body; an optional `id`
  query parameter (validated with `validID`) reuses a specific subid, otherwise
  one is generated (`randomHex(16)`). If a subscription with that id already
  exists → `409` `{"success":false}` (it won't clobber existing data). Returns
  `201` `{"success":true,"id":"..."}`. Peers are added afterwards via
  `/api/update`. An empty `subs.yaml` renders the "No configurations yet." page.
- `POST /api/update?id=<id>` (`handleUpdate`) — appends a peer to an existing
  subscription. Multipart form: `title`/`config_file` required,
  `comment`/`link`/`qr`/`exclusive` optional (parsed by the shared `parsePeerUpload`
  helper, except `exclusive` read in the handler; body capped via
  `http.MaxBytesReader`, oversize → `413`). `id` comes from the query string and is
  validated with `validID`. If the subscription's `subs.yaml` is missing →
  `404` `{"success":false}`; otherwise saves the uploaded file under its own name
  and, by default, appends the `Peer` (`appendPeer` in peer.go). `exclusive=true`
  instead replaces the whole peer list with just this peer (`writeClient`).
  Returns `200` `{"success":true}`.
- `POST /api/delete?id=<id>` (`handleDelete`) — removes the subscription directory
  `work_dir/<id>/` and all of its contents (`os.RemoveAll`). `id` comes from the
  query string and is validated with `validID`. If the directory does not exist →
  `404` `{"success":false}`; otherwise `200` `{"success":true}`.
- `GET /api/check?subid=<id>` (`handleCheck`) — reports whether
  `work_dir/<subid>/subs.yaml` exists. Returns `200`
  `{"success":true,"exists":true}` or `{"success":true,"exists":false}`.

## Request flow (server.go)

- Only `GET` is allowed; anything else → 405.
- The path is `/<uuid>` (page) or `/<uuid>/<config_file>` (download); more than two
  segments → 404. `validID` rejects traversal (`/`, `\`, `..`) on each segment.
- `subs.yaml` (`peersFile`) — `Client` in peer.go: a flat top-level list of
  `Peer` (title, comment, link, config_file, qr).
- `renderPage` renders `pageTemplate` (template.go): centered config `title` (the
  optional config `description` appears as a CSS tooltip ~500ms after hovering the
  title), one
  card per peer (`title` over `comment`) with Download / Copy link / QR buttons.
  Copy link and QR (modal) are client-side JS. The QR button is always rendered,
  but disabled when `Peer.ShowQR()` is false (the `qr` field defaults to true;
  `qr: false` greys the button out).
- `serveConfig` serves `/<uuid>/<config_file>`, but only for names returned true by
  `Client.hasConfigFile` (i.e. declared in `subs.yaml`). With `?qr` it returns a
  PNG QR of the file contents (`serveQR`, via `github.com/skip2/go-qrcode`), but
  only when `Client.qrAllowed(name)` is true (peer `qr` not set to false),
  otherwise `404`; without `?qr` the file is sent as a download.
- Page response headers come from the config `headers:` block. `Content-Type`
  defaults to `text/html` but a config header of the same name overrides it.

## Files

- `main.go` — entry point, config loading, page + API server start.
- `config.go` — config model, config-file lookup, listen-address helpers.
- `peer.go` — `subs.yaml` model, loader, and `appendPeer`.
- `template.go` — HTML template and view model.
- `server.go` — public GET handler, ID validation.
- `api.go` — authenticated management API (`POST /api/create`).
