# subs

A small Go HTTP service that serves per-UUID VPN subscription pages.

A client requests `http://host/<uuid>`. The service looks up `work_dir/<uuid>/`,
parses the `subs.yaml` peer list found there, and renders it as an HTML page
listing every peer (title, comment, link, config file).

## Build

```sh
make build         # local system        -> .bin/subs
make build-linux   # linux/amd64          -> .bin/subs-linux-amd64
make build-all     # both
```

## Run

```sh
subs                       # uses ./.subs.yaml, then ~/.subs.yaml, then /etc/subs.yaml
subs path/to/config.yaml   # explicit config file
subs -h, --help            # show usage
subs -v, --version         # show version
```

The server listens on the configured `port` (default `9876`) and handles **GET**
requests only (anything else returns `405`).

### systemd

A hardened example unit is provided in [`subs.service`](subs.service). It runs as
the `subs` user/group, reads `/etc/subs.yaml`, and uses `StateDirectory=subs` to
create and own `/var/lib/subs` (set `work_dir: /var/lib/subs` in the config).

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin subs
sudo install -m 0755 .bin/subs /usr/local/bin/subs
sudo install -m 0640 -o subs -g subs your-config.yaml /etc/subs.yaml
sudo cp subs.service /etc/systemd/system/subs.service
sudo systemctl daemon-reload
sudo systemctl enable --now subs
```

## Configuration

Config is YAML. Lookup precedence: the path passed as an argument, then
`./.subs.yaml`, then `~/.subs.yaml`, then `/etc/subs.yaml`.

```yaml
host: 127.0.0.1        # optional, defaults to 127.0.0.1
port: 9876             # optional, defaults to 9876
work_dir: .subs        # optional, defaults to ".subs"
title: Hello World!    # optional, rendered as the page <title> and heading
description: ...        # optional, shown as a tooltip when hovering the heading
headers:               # optional, added to every response
  title: hello world!
api_enabled: false     # optional, defaults to false
api_host: 127.0.0.1    # optional, defaults to 127.0.0.1
api_port: 4321         # optional, defaults to 4321
api_token: change-me   # required when api_enabled is true (bearer token)
max_upload_mb: 10      # optional, defaults to 10 (POST /api/update body limit)
```

- `host` / `api_host` — bind addresses. Both default to `127.0.0.1`, which is the
  intended setup behind a TLS-terminating reverse proxy (e.g. nginx). Set to
  `0.0.0.0` to listen on all interfaces.
- `work_dir` — directory holding one subdirectory per UUID. Relative paths are
  resolved against the directory containing the config file. It **must exist** at
  startup, otherwise the service prints an error and exits.
- `api_*` — the management API (see below). When `api_enabled` is true,
  `api_token` must be set, otherwise the service exits at startup.
- `max_upload_mb` — maximum `POST /api/create` request body size; larger requests get
  `413`.

## Management API

When `api_enabled: true`, an authenticated API listens on `api_port` (default
`4321`), separate from the public page server. All requests require an
`Authorization: Bearer <api_token>` header.

Every response carries a `success` field. On success the body is
`{"success":true, ...}` (with any endpoint-specific fields); on any failure the
body is just `{"success":false}` with an appropriate HTTP status code.

| Endpoint            | Success                                       | Failure              |
|---------------------|-----------------------------------------------|----------------------|
| `POST /api/create`  | `201` `{"success":true,"id":"a1s2d3f4"}`      | `{"success":false}`  |
| `POST /api/update`  | `200` `{"success":true}`                      | `{"success":false}`  |
| `POST /api/delete`  | `200` `{"success":true}`                      | `{"success":false}`  |
| `GET /api/check`    | `200` `{"success":true,"exists":true/false}`  | `{"success":false}`  |

### `POST /api/create`

Creates an empty subscription: `work_dir/<subid>/` with an empty `subs.yaml`.
Takes no body. An optional `id` query parameter reuses a specific subid
(validated against path traversal); otherwise one is generated (`openssl rand
-hex 16`). Peers are added afterwards with [`/api/update`](#post-apiupdateidid).

```sh
curl -X POST https://domain.com:4321/api/create \
  -H "Authorization: Bearer YOUR_TOKEN"
```

On success returns `201` with `{"success":true,"id":"<id>"}`. The subscription
page is then served at `http://host:<port>/<subid>` and, until a peer is added,
shows a "No configurations yet." placeholder. If the id already exists the
request fails with `409` `{"success":false}` rather than clobbering it.

### `POST /api/update?id=<id>`

Multipart form that appends a peer to the existing subscription `<id>`. The
uploaded file is stored under its own name in `work_dir/<id>/` and a new entry is
appended to `subs.yaml`.

Fields:

| Field         | Required | Notes                                            |
|---------------|----------|--------------------------------------------------|
| `config_file` | yes      | uploaded file; saved as-is, name used in yaml    |
| `title`       | yes      | peer title                                       |
| `comment`     | no       | peer comment                                     |
| `link`        | no       | `vpn://...`                                       |
| `qr`          | no       | bool; default `true`. `false` disables (greys out) the peer's QR button on the page |
| `exclusive`   | no       | bool; default `false` (append). `true` replaces the whole peer list with just this peer |

```sh
curl -X POST "https://domain.com:4321/api/update?id=<id>" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "config_file=@/path/to/server.conf;type=text/plain" \
  -F "title=Germany" \
  -F "comment=device 1" \
  -F "link=vpn://zxcv"
```

Returns `200` with `{"success":true}`. If the subscription does not exist →
`404` `{"success":false}`.

### `POST /api/delete?id=<id>`

Deletes a subscription, removing `work_dir/<id>/` and everything inside it.

```sh
curl -X POST "https://domain.com:4321/api/delete?id=<id>" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Returns `200` with `{"success":true}` when removed, or `404` with
`{"success":false}` when no such subscription exists.

### `GET /api/check?subid=<id>`

Reports whether a subscription exists (a `work_dir/<subid>/subs.yaml`).

```sh
curl "https://domain.com:4321/api/check?subid=<id>" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Returns `200` with `{"success":true,"exists":true}` when present, or
`{"success":true,"exists":false}` otherwise.

## Work directory layout

For a request to `/<uuid>`, the service reads `work_dir/<uuid>/`:

```
work_dir/
  <uuid>/
    subs.yaml      # required — the peer list to render
```

### `subs.yaml`

A flat list of peers. Every field is shown on the page. `config_file`
values are file names within the same `work_dir/<uuid>/` directory.

```yaml
- title: Germany
  comment: device 1
  link: vpn://...
  config_file: gr.conf
```

If this file is missing, the request returns `404`.

The rendered page centers the config `title` and shows one card per peer with its
`title` and `comment`, plus action buttons: **Download config**, **Copy link**
(copies the peer `link` to the clipboard), and **QR code** (shows a QR of the
config file contents in a modal).

Each peer's config file is downloadable at `GET /<uuid>/<config_file>`; the same
path with `?qr` returns a PNG QR code of the file contents. Only files declared as
a `config_file` in `subs.yaml` are served.

## Responses

| Situation | Status |
|-----------|--------|
| Valid UUID with `subs.yaml` | `200` (HTML) |
| `GET /<uuid>/<config_file>` for a declared file | `200` (download) |
| `GET /<uuid>/<config_file>?qr` | `200` (PNG QR code) |
| Unknown UUID / missing directory | `404` |
| Directory without `subs.yaml` | `404` |
| Non-GET method | `405` |
