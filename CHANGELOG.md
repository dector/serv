# Changelog

## [0.8.3-snapshot] - Unreleased

### Fixed

- Keep default and explicit local ports when Tailscale Serve occupies matching tailnet ports.

## [0.8.2] - 2026-08-01

### Changed

- Streamline Tailscale exposure output to print the tailnet HTTPS URL with regular launch info.
- Add `--verbose` to show full Tailscale Serve output when needed.

## [0.8.1] - 2026-08-01

### Added

- Add `--expose-tailscale`/`-T` to expose the served content through Tailscale Serve.

## [0.8.0] - 2026-08-01

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

