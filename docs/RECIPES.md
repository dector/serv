# RECIPES

Practical examples for all `serv` features and options.

## Install

```sh
mise use -g github:dector/serve
serv --version
```

## Show version

```sh
serv --version
# or
serv -v
```

## Serve a single file

```sh
serv README.md
```

By default, `serv` uses preview mode. Supported Markdown files render as HTML; use `?raw=1` to view the raw file.

Open the printed URL in your browser (for example `http://localhost:8080`).

## Set a specific port

```sh
serv --port 3000 Downloads
# or
serv -p 3000 Downloads
```

If that port is busy, `serv` will automatically pick another free port.

## Open the served URL in your browser

```sh
serv --open README.md
# or
serv -o Downloads
```

`--open`/`-o` opens the served URL in your default browser after the server starts.
Browser opening is best-effort; if it fails, the server keeps running and prints the URL.

## Serve a directory

```sh
serv Downloads
```

In default preview mode, directories resolve `README` first, then `index.html`, then fall back to a listing page.

### Use file browsing mode

Use file mode to serve files raw and always show directory listings by default:

```sh
serv --mode file Downloads
# or
serv -m f Downloads
```

### Change directory resolution

Choose an explicit directory strategy:

```sh
serv --mode preview --dir-resolve index-first site
serv -m p --dir-resolve rf .
serv -m file --dir-resolve index-only site
```

Available strategies: `readme-first`/`rf`, `index-first`/`if`, `readme-only`/`ro`, `index-only`/`io`, `none`/`n`.

Per-request overrides are available too:

```text
/docs/?resolve=none
/docs/?resolve=index-only
```

### File preview overrides

```text
/README.md?raw=1
/README.md?preview=1
```

`raw=1` wins when both `raw=1` and `preview=1` are present.

