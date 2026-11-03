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

Translation goes through [EveryAPI](https://everyapi.ai) by default, using the account already signed in with `everyapi auth login`. The app asks the CLI for the relay key (`everyapi auth credential`), so a desktop launch picks up the same login, gateway region, and key refresh the CLI uses. No separate token is required.

The app reads its settings from the environment:

- `LINGOCUT_WHISPER_MODEL` (required) — path of a whisper.cpp ggml model, such as `ggml-small.bin`.
- `LINGOCUT_LLM_MODEL` — translation model, default `claude-haiku-5-5` on EveryAPI; required with a custom provider. `everyapi models list` shows what EveryAPI offers.
- `LINGOCUT_LLM_BASE_URL` — set it to use a custom OpenAI-compatible provider instead of EveryAPI, such as `https://api.openai.com/v1` or `http://localhost:11434/v1` for Ollama. The signed-in EveryAPI account is not used then.
- `LINGOCUT_LLM_API_KEY` or `EVERYAPI_API_KEY` — optional bearer token that overrides the signed-in account.
- `LINGOCUT_OUTPUT_DIR` — where videos and subtitles go, default `~/Movies/LingoCut` on macOS and `~/Videos/LingoCut` elsewhere.
- `LINGOCUT_GOOGLE_CLIENT_ID` — Google OAuth client id for the account page. It is not saved. `LINGOCUT_GOOGLE_CLIENT_SECRET` is optional.
- `LINGOCUT_R2_ENDPOINT`, `LINGOCUT_R2_BUCKET`, `LINGOCUT_R2_ACCESS_KEY`, `LINGOCUT_R2_SECRET` — Cloudflare R2, used when set instead of the values saved in settings. The endpoint may be an account id or a full URL.
- `LINGOCUT_S3_REGION`, `LINGOCUT_S3_BUCKET`, `LINGOCUT_S3_ACCESS_KEY`, `LINGOCUT_S3_SECRET` — Amazon S3. The region is a name such as `ap-northeast-1`. `LINGOCUT_STORAGE=s3` selects S3.
- `LINGOCUT_OSS_REGION`, `LINGOCUT_OSS_BUCKET`, `LINGOCUT_OSS_ACCESS_KEY`, `LINGOCUT_OSS_SECRET` — Alibaba Cloud OSS. The region is a name such as `cn-hangzhou`. `LINGOCUT_STORAGE=oss` selects OSS.
- `LINGOCUT_COS_REGION`, `LINGOCUT_COS_BUCKET`, `LINGOCUT_COS_ACCESS_KEY`, `LINGOCUT_COS_SECRET` — Tencent Cloud COS. The region is a name such as `ap-guangzhou`, and the bucket name includes the APPID. `LINGOCUT_STORAGE=cos` selects COS.
- `LINGOCUT_S3COMPAT_ENDPOINT`, `LINGOCUT_S3COMPAT_REGION`, `LINGOCUT_S3COMPAT_BUCKET`, `LINGOCUT_S3COMPAT_ACCESS_KEY`, `LINGOCUT_S3COMPAT_SECRET` — any other S3-compatible store. `LINGOCUT_S3COMPAT_PATH_STYLE=1` turns on path-style addressing, which MinIO needs. `LINGOCUT_STORAGE=compat` selects it. Otherwise the upload goes to R2.

Finished and in-progress jobs are listed in the sidebar and remembered in `library.json` under the user config directory (`~/Library/Application Support/LingoCut` on macOS).

Rate limits and server errors from the provider are retried up to three times.
