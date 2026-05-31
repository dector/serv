# serv

Serve file or folder as a web-page.

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

## Behavior

- You can serve either a directory or a single file.
- If a requested directory contains `index.html`, it is served automatically.
- If no `index.html` is found, `serv` renders a directory listing page.
- Use `--no-index-resolve` to disable automatic `index.html` resolution in directories.

``` shell
serv --no-index-resolve Downloads
```

# License

Discributed using [MIT](https://opensource.org/license/mit) license.

