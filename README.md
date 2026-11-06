# Kando (看懂)

A native desktop video transcription and translation app built with Go and MyGo.

## Repository layout

- `apps/kando` — native UI application and platform packaging entry point.
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

`make dev` runs MyGo's `mygo dev` patched by `tools/mygo-dev`: on macOS, a reload keeps the running window until the new build shows its own, instead of quitting it first, so saving a change does not look like the app quitting. If the patch stops applying after a MyGo upgrade, it warns and falls back to the stock `go tool mygo dev`; update `tools/mygo-dev/dev.go.patch` against the new `cmd/mygo/dev.go` then.

## Media pipeline

A job downloads the video with `yt-dlp`, extracts audio with `ffmpeg`, transcribes it locally with whisper.cpp (`whisper-cli`), translates the lines through an OpenAI-compatible chat API, and writes `<video>.srt` and `<video>.bilingual.srt` next to the video. Install the tools first, for example `brew install yt-dlp ffmpeg whisper-cpp` on macOS.

Translation goes through [EveryAPI](https://everyapi.ai) by default, using the account already signed in with `everyapi auth login`. The app asks the CLI for the relay key (`everyapi auth credential`), so a desktop launch picks up the same login, gateway region, and key refresh the CLI uses. No separate token is required.

The app reads its settings from the environment:

- `KANDO_WHISPER_MODEL` (required) — path of a whisper.cpp ggml model, such as `ggml-small.bin`.
- `KANDO_LLM_MODEL` — translation model, default `claude-haiku-5-5` on EveryAPI; required with a custom provider. `everyapi models list` shows what EveryAPI offers.
- `KANDO_LLM_BASE_URL` — set it to use a custom OpenAI-compatible provider instead of EveryAPI, such as `https://api.openai.com/v1` or `http://localhost:11434/v1` for Ollama. The signed-in EveryAPI account is not used then.
- `KANDO_LLM_API_KEY` or `EVERYAPI_API_KEY` — optional bearer token that overrides the signed-in account.
- `KANDO_OUTPUT_DIR` — where videos and subtitles go, default `~/Movies/影视库.kando` on macOS and `~/Videos/影视库.kando` elsewhere.
- `KANDO_GOOGLE_CLIENT_ID` — Google OAuth client id for the account page. It is not saved. `KANDO_GOOGLE_CLIENT_SECRET` is optional.
- `KANDO_R2_ENDPOINT`, `KANDO_R2_BUCKET`, `KANDO_R2_ACCESS_KEY`, `KANDO_R2_SECRET` — Cloudflare R2, used when set instead of the values saved in settings. The endpoint may be an account id or a full URL.
- `KANDO_S3_REGION`, `KANDO_S3_BUCKET`, `KANDO_S3_ACCESS_KEY`, `KANDO_S3_SECRET` — Amazon S3. The region is a name such as `ap-northeast-1`. `KANDO_STORAGE=s3` selects S3.
- `KANDO_OSS_REGION`, `KANDO_OSS_BUCKET`, `KANDO_OSS_ACCESS_KEY`, `KANDO_OSS_SECRET` — Alibaba Cloud OSS. The region is a name such as `cn-hangzhou`. `KANDO_STORAGE=oss` selects OSS.
- `KANDO_COS_REGION`, `KANDO_COS_BUCKET`, `KANDO_COS_ACCESS_KEY`, `KANDO_COS_SECRET` — Tencent Cloud COS. The region is a name such as `ap-guangzhou`, and the bucket name includes the APPID. `KANDO_STORAGE=cos` selects COS.
- `KANDO_S3COMPAT_ENDPOINT`, `KANDO_S3COMPAT_REGION`, `KANDO_S3COMPAT_BUCKET`, `KANDO_S3COMPAT_ACCESS_KEY`, `KANDO_S3COMPAT_SECRET` — any other S3-compatible store. `KANDO_S3COMPAT_PATH_STYLE=1` turns on path-style addressing, which MinIO needs. `KANDO_STORAGE=compat` selects it. Otherwise the upload goes to R2.

Finished and in-progress jobs are listed in the sidebar and remembered in `library.json` under the user config directory (`~/Library/Application Support/Kando` on macOS).

Rate limits and server errors from the provider are retried up to three times.

## Upgrading from the old name

Nothing reads the old names. Quit the app first. Then rename the config folder to `Kando`; it holds `library.json`, `settings.json` and the downloaded models. Next, rename each library's suffix to `.kando`. Do this after renaming the config folder and before launching, so the saved inodes let the app follow the renamed libraries. Then rename each environment variable's prefix to `KANDO_`. Finally, delete the old app bundle and its poster cache. Old to new: `~/Library/Application Support/LingoCut` (`%AppData%\LingoCut` on Windows, `~/.config/LingoCut` on Linux) to `Kando`, `X.lingocut` to `X.kando`, `LINGOCUT_*` to `KANDO_*`, and delete `lingocut.app` and `~/Library/Caches/LingoCut`.
