# LingoCut

A native desktop video transcription and translation app built with Go and MyGo.

## Repository layout

- `apps/lingocut` — native UI application and platform packaging entry point.
- `packages/core` — platform-independent jobs, states, and domain contracts.
- `packages/media` — media pipeline boundary for download, transcription, translation, and export.
- `platforms/darwin` — macOS-specific integrations.
- `platforms/windows` — Windows-specific integrations.
- `platforms/linux` — Linux-specific integrations.
- `tools` — repository development tools.

The app uses MyGo's native UI. It does not embed a webview.

## Development

Requires Go 1.27.3+.

```sh
make test
make dev
```

The current MVP contains the native shell and URL validation. Media processing will be added behind the `packages/media` boundary so the same core can ship on macOS, Windows, and Linux.
