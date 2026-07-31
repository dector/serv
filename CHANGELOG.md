# Changelog

## [0.7.1-snapshot] - Unreleased

Changelog tracking started.

### BREAKING CHANGES

- Default mode is now `preview`.
- Default directory behavior is now README-first preview resolution instead of automatic `index.html` resolution.
- Markdown renders by default in preview mode.
- File browsing behavior is available with `--mode file` / `-m f`.
- `--browser`/`-B` is deprecated and will be removed in a future release. Use `--open`/`-o` instead.

### Deprecated

- `--preview`/`-P` is deprecated; use `--mode preview`.
- `--no-index-resolve` is deprecated; use `--dir-resolve` strategies.

