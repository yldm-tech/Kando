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

Translation goes through [EveryAPI](https://everyapi.ai) by default. Create a relay token for LingoCut and export its key:

```sh
everyapi token create --name lingocut
everyapi token key <id>   # prints sk-everyapi-...
export EVERYAPI_API_KEY=sk-everyapi-...
```

The app reads its settings from the environment:

- `LINGOCUT_WHISPER_MODEL` (required) — path of a whisper.cpp ggml model, such as `ggml-small.bin`.
- `EVERYAPI_API_KEY` — EveryAPI relay key, required when using EveryAPI.
- `LINGOCUT_LLM_MODEL` — translation model, default `claude-haiku-5-5` on EveryAPI; required with a custom provider. `everyapi models list` shows what EveryAPI offers.
- `LINGOCUT_LLM_BASE_URL` — set it to use a custom OpenAI-compatible provider instead of EveryAPI, such as `https://api.openai.com/v1` or `http://localhost:11434/v1` for Ollama. EveryAPI's default key and model are not used then.
- `LINGOCUT_LLM_API_KEY` — bearer token for the provider; takes precedence over `EVERYAPI_API_KEY`.
- `LINGOCUT_OUTPUT_DIR` — where videos and subtitles go, default `~/Movies/LingoCut` on macOS and `~/Videos/LingoCut` elsewhere.

Rate limits and server errors from the provider are retried up to three times.
