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

## Media pipeline

A job downloads the video with `yt-dlp`, extracts audio with `ffmpeg`, transcribes it locally with whisper.cpp (`whisper-cli`), translates the lines through an OpenAI-compatible chat API, and writes `<video>.srt` and `<video>.bilingual.srt` next to the video. Install the tools first, for example `brew install yt-dlp ffmpeg whisper-cpp` on macOS.

The app reads its settings from the environment:

- `LINGOCUT_WHISPER_MODEL` (required) — path of a whisper.cpp ggml model, such as `ggml-small.bin`.
- `LINGOCUT_LLM_MODEL` (required) — chat model used for translation.
- `LINGOCUT_LLM_BASE_URL` — API base URL, default `https://api.openai.com/v1`; use `http://localhost:11434/v1` for Ollama.
- `LINGOCUT_LLM_API_KEY` — bearer token, if the API needs one.
- `LINGOCUT_OUTPUT_DIR` — where videos and subtitles go, default `~/Movies/LingoCut` on macOS and `~/Videos/LingoCut` elsewhere.
