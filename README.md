# serv

Serve file or folder as a web-page.

# Usage

## Install

``` shell
mise install ubi:dector/serve
mise use -g ubi:dector/serve
serve --version
```

## Serve file or folder

``` shell
serv README.md
serv Downloads
```

Default port: `8080`.

To use port - pass `--port`/`-p`:

``` shell
serv -p 3000 README.md
```

# License

Discributed using [MIT](https://opensource.org/license/mit) license.

