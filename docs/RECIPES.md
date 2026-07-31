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

This serves that file directly.

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

When serving a directory, `serv` will try to find and serve `index.html` in the requested directory first.
If no `index.html` is found, it renders a directory listing page.

### Directory behavior: auto `index.html`

If a directory contains `index.html`, `serv` serves it automatically:

```sh
serv site/
```

For `site/docs/`, if `site/docs/index.html` exists, that file is served.

### Disable `index.html` auto-resolution

Use this to always show listing pages for directories, even when `index.html` exists:

```sh
serv --no-index-resolve site/
```

### Directory behavior: listing page fallback

If no `index.html` exists in the requested directory, `serv` renders a directory listing page automatically:

```sh
serv Downloads
```

