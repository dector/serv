# serv

Serve file or folder as a web-page.

[CHANGELOG](./CHANGELOG.md)

# Usage

## Install

``` shell
mise install github:dector/serve
serve --version
```

## Serve file or folder

``` shell
serv README.md
serv Downloads
```

Default port: `8080`.

To use a specific port, pass `--port`/`-p`:

``` shell
serv -p 3000 README.md
```

If the requested port is unavailable, `serv` automatically picks another free port.

## Open in browser

Use `--open`/`-o` to open the served URL in your default browser after the server starts:

``` shell
serv --open README.md
serv -o Downloads
```

Browser opening is best-effort; if it fails, the server keeps running and prints the URL.

## Serving modes

`serv` defaults to preview mode, optimized for reading repos and websites:

``` shell
serv .
serv README.md
```

In preview mode, supported Markdown files (`.md`, `.markdown`, `.mdown`, `.mkd`) render as HTML. Directories resolve `README` first, then `index.html`, then fall back to a listing.

Use file mode for raw file browsing:

``` shell
serv --mode file Downloads
serv -m f Downloads
```

In file mode, files are served raw and directories show listings by default.

Change directory resolution with `--dir-resolve`:

``` shell
serv --mode preview --dir-resolve index-first site
serv -m p --dir-resolve rf .
serv -m file --dir-resolve index-only site
```

Strategies: `readme-first`/`rf`, `index-first`/`if`, `readme-only`/`ro`, `index-only`/`io`, `none`/`n`.

Per-request URL overrides:

```text
/README.md?raw=1
/README.md?preview=1
/docs/?resolve=none
/docs/?resolve=index-only
```

## Behavior

- You can serve either a directory or a single file.
- HTML files are served directly as static HTML.
- Missing directory resolution candidates fall back to a directory listing.
- `--preview`/`-P` is deprecated; use `--mode preview`.
- `--no-index-resolve` is deprecated; use `--dir-resolve`.

# License

Discributed using [MIT](https://opensource.org/license/mit) license.

