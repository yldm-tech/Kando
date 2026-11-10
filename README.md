# Kando

[![CI](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml)
[![Release](https://github.com/yldm-tech/Kando/actions/workflows/release.yml/badge.svg)](https://github.com/yldm-tech/Kando/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/yldm-tech/Kando?include_prereleases&sort=semver)](https://github.com/yldm-tech/Kando/releases)
[![License](https://img.shields.io/github/license/yldm-tech/Kando)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/yldm-tech/Kando?filename=apps%2Fkando%2Fgo.mod)](go.work)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows%20%28from%20source%29-lightgrey)](#install)
[![GitHub stars](https://img.shields.io/github/stars/yldm-tech/Kando)](https://github.com/yldm-tech/Kando/stargazers)

English · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [繁體中文](README.zh-TW.md) · [한국어](README.ko.md)

Kando is a desktop app for learning a language from videos in that language. Paste a YouTube or Bilibili link, or drop a video file on the window: Kando downloads the video with yt-dlp, transcribes the speech on your own computer with whisper.cpp, translates each line through an OpenAI-compatible model, and plays the original video with the original line and its translation on screen together. It is written in Go and draws its native interface with [MyGo](https://github.com/egoist/mygo) and [Breeze UI](https://github.com/yldm-tech/mygo-breeze-ui); there is no web view.

![The library: the most recent film, the jobs in progress and the films in the library](docs/assets/en/library.png)

The interface is in English, Simplified Chinese, Traditional Chinese, Japanese and Korean. Kando starts in Simplified Chinese; choose another language in Settings › General › Interface language, and the window changes at once. The labels quoted below are those of the English interface.

## Features

- Add a video by pasting a link from YouTube, Bilibili or any other site yt-dlp supports (Help › Supported Websites… lists them, with their logos), or pick a local file (mp4, mov, mkv, webm and other video files) or drag it onto the window. Local files are transcribed where they are, not copied.
- Jobs run one after another through download, transcription, translation and export. A failed job says why, links to the setting that fixes it, and can be retried or removed.
- Speech is recognized on your computer with whisper.cpp, which detects the spoken language. Settings › Speech Recognition lists seven whisper models, from Tiny (77.7 MB) to Large v3 (3.1 GB), and downloads the one you choose from Hugging Face.
- Lines are translated by EveryAPI by default, or by any OpenAI-compatible chat API: Settings › AI Service has presets for OpenAI, OpenRouter, Groq, DeepSeek, xAI, Mistral, Qwen (DashScope), Kimi, Zhipu GLM, SiliconFlow, Ollama and LM Studio, plus a custom endpoint. The target language, a context-aware mode that lets the model read the preceding lines, and the prompt can be changed in Settings › Translation.
- Each finished video gets two SubRip files next to it: `<video>.srt` with the original lines and `<video>.bilingual.srt` with each translation above its original line (`video.srt` and `video.bilingual.srt` for a downloaded video).
- The player shows the subtitles as bilingual, original only, translation only or off, with a delay you can adjust in 0.1-second steps and a style you can change. Its side panel lists every line with search (click a line to jump to it), splits the video into 4 to 12 clips of about 30 seconds, and shows the film's files and source. Each film resumes where you stopped watching.
- Libraries are `.kando` folders of ordinary files you can manage in Finder or a file manager. You can keep several and switch between them from the sidebar, which also filters a library by source: YouTube, Bilibili, other sites and local files.
- Optional upload of a film's video and bilingual subtitles to your own object storage (Cloudflare R2, Amazon S3, Alibaba Cloud OSS, Tencent Cloud COS or any S3-compatible store), from the player, after signing in with Google. See [Configuration](#configuration).
- Keyboard shortcuts for playback, subtitles and windows, listed in a cheat sheet (⌘/) and in Settings › Keyboard Shortcuts.

![The player with bilingual subtitles and the side panel's list of lines](docs/assets/en/player.png)

## Install

### Download

Builds for macOS (Apple silicon and Intel) and Linux (x86-64 and ARM64) are on [GitHub Releases](https://github.com/yldm-tech/Kando/releases); there is no Windows build yet, so on Windows build from source as described below. Pick the file for your system, where `<version>` is the release's version, such as `0.5.0`:

- macOS: `kando-<version>-arm64.dmg` for Apple silicon, `kando-<version>-amd64.dmg` for Intel. Open the disk image and drag Kando to Applications.
- Debian, Ubuntu and their derivatives: `kando_<version>_amd64.deb` for x86-64, `kando_<version>_arm64.deb` for ARM64. Install it with `sudo apt install ./kando_<version>_<arch>.deb` in the folder you downloaded it to.
- Other Linux distributions: run the release's install script, which downloads the package for you:

  ```sh
  curl -fsSL https://github.com/yldm-tech/Kando/releases/latest/download/install.sh | sh
  ```

  It downloads the latest `kando-<version>-linux-<arch>.tar.gz` for your machine, checks its signature, and installs Kando for your user alone, without root: the app in `~/.local/kando.app`, the `kando` command in `~/.local/bin`, and an entry in the applications menu. Checking the signature needs OpenSSL 3.0 or later (Debian 12, Ubuntu 22.04, Fedora 36 and later); with an older OpenSSL the script downloads nothing and says so. In that case, download the `.tar.gz` for your machine and `install.sh` from the same release into one folder, keeping their file names, and run `sh install.sh` there, or run `sh install.sh <archive>` with the path of the `.tar.gz`; either way the script installs that archive without checking its signature. Running the script again installs the newer version over the old one, and `sh install.sh --uninstall` removes Kando and keeps its settings and libraries.

The other files of a release are not for installing: `checksums.txt` lists the SHA-256 of every file, and the `update-*.json` manifests, the `.delta` files and the `darwin` archives are what installed copies of Kando update themselves from (see [Updates](#updates)).

The macOS disk images of releases newer than 0.5.1 are signed with YLDM's Developer ID and notarized by Apple, so macOS opens Kando without a warning.

### Updates

Kando updates itself from the signed releases on GitHub. It checks in the background 10 seconds after it opens and then once a day; when a new version is out, a window shows its release notes and offers Install Update, Remind Me Later and Skip This Version. Install Update downloads the new version, checks its signature and offers to relaunch Kando into it; otherwise it opens the next time you launch Kando. A version you skipped is not offered again by the background checks, but the next one is.

To check now, choose Kando › Check for Updates… in the menu bar on macOS, or Check for Updates… in the command palette (Ctrl+K) on Linux and Windows, which have no menu bar; Settings › General also has a Check for Updates… button next to Version, under which it shows the newer version the last check found or when Kando last checked. The switches Check for updates automatically and Download and install updates automatically are in Settings › General too. The second shows only while the first is on, as only the background checks download updates; with it on, they install a new version without asking, and it opens the next time you launch Kando.

A copy of Kando that cannot replace its own files does not update itself. That is the case of the `.deb` and the `PKGBUILD`, which install Kando in `/opt/kando`, a folder the package manager owns: there is no apt repository for Kando, so to update, download the new `.deb` from the release and install it the same way, or move to the install script above, whose copy in `~/.local/kando.app` updates itself. In such a copy Settings › General shows no switches, and Check for Updates… opens a window that says why it cannot update. Windows has no published updates yet.

### Build from source

With Go 1.27.3 or later:

```sh
git clone https://github.com/yldm-tech/Kando.git
cd Kando
make build
```

`make build` runs `go tool mygo build apps/kando` for the platform you are on and writes the result under `apps/kando/build/<os>-<arch>/`: on macOS an ad-hoc signed `Kando.app` and a `.dmg`, on other platforms an executable (Linux also gets a `.deb`, a `.tar.gz` with an install script and a `PKGBUILD`; Windows an installer made with NSIS, which mygo downloads when it is not installed). Without `make`, as on a stock Windows, run `go tool mygo build apps/kando` from the repository root.

## Requirements

Kando runs `yt-dlp`, `ffmpeg` and `whisper-cli` (whisper.cpp). On macOS and Linux there is nothing to install: when Kando opens and one of them is missing, it downloads it, and the Getting Started guide shows the progress. Programs you already installed, such as with Homebrew, are used as they are.

| Program | Used for | What Kando downloads |
| --- | --- | --- |
| `yt-dlp` | Downloading online videos. Kando asks for it for every job, local files included. | The standalone build from the [yt-dlp release](https://github.com/yt-dlp/yt-dlp/releases). |
| `ffmpeg` | Extracting the audio, grabbing covers, and decoding the picture and the sound in the player, which plays the sound through the system's audio output on macOS, Linux and Windows. | FFmpeg 9.0.2, with `ffplay` on macOS, from this repository's [tools release](https://github.com/yldm-tech/Kando/releases/tag/tools-1). |
| `whisper-cli` (whisper.cpp) | Speech recognition. | whisper.cpp 1.9.5, with Metal on macOS, from the same release. |
| A whisper.cpp ggml model | Download one in the Getting Started guide or Settings › Speech Recognition, or point `KANDO_WHISPER_MODEL` at a model file. | |

The tools release is built by the Tools workflow (`.github/workflows/tools.yml`) from the sources pinned in `.github/tools.env`, and Kando checks each download against a SHA-256 pinned in the app before using it. Downloads go to the `tools` folder beside `settings.json` (`~/Library/Application Support/Kando/tools` on macOS, `~/.config/Kando/tools` on Linux). Kando looks there first, then on `PATH` and in `/opt/homebrew/bin`, `/usr/local/bin` and `/home/linuxbrew/.linuxbrew/bin`, because apps opened from Finder or a desktop launcher get a minimal `PATH`. Settings › General shows which program is used, offers Download or Retry for one that is missing, and Detect Again looks again. If a download fails because `yt-dlp` is out of date, Kando offers Update and Retry, which updates it the way it was installed and retries.

On Windows Kando does not download them yet: install the same programs with your package manager and make sure they are on `PATH`.

## Getting started

1. Open Kando. With no library yet, it offers New Library… (named Film Library in `~/Movies` on macOS or `~/Videos` elsewhere unless you change it) and Open Existing Library….
2. The Getting Started guide downloads `ffmpeg`, `yt-dlp` and `whisper-cli` if they are missing. Open Settings (⌘,): General shows them with their versions, and has the interface language.
3. In Speech Recognition, click Download next to a whisper model; Kando selects it when the download finishes. The one marked Recommended is Large v3 Turbo (Compressed), 574 MB. Models that Vimpo or hyperframes already downloaded can be selected without copying.
4. In AI Service, set up translation:
   - EveryAPI (the default): sign in once with the [EveryAPI](https://everyapi.ai) CLI, `everyapi auth login`. Kando asks the CLI for the key (`everyapi auth credential`), so it uses the same login, gateway region and key refresh as the CLI, and you do not paste a token. The default model is `claude-haiku-5-5`; `everyapi models list` shows the others. You can also paste an EveryAPI key into API Key instead.
   - Another provider: choose it under Service, or choose OpenAI Compatible and enter its Base URL, then fill in Model and API Key (local servers such as Ollama and LM Studio need no key). Test Connection sends a short request to check the settings.
5. In Translation, pick the default target language: Simplified Chinese (the default), Traditional Chinese, English, Japanese, Korean, Spanish, French or German.
6. Back in the window, paste a link into the top bar and click Start Processing, or click Local Video or drag a video onto the window. The status line at the bottom of the sidebar says Ready to process when everything a job needs is in place, and otherwise names what is missing.

When the translation provider answers with a rate limit or a server error, Kando sends the request again, up to three attempts in all.

![Settings › General: the open library, the interface language, the version and the programs Kando found](docs/assets/en/settings-general.png)

![Settings › AI Service with EveryAPI selected](docs/assets/en/settings-ai.png)

## Command line and MCP

The Kando program is also a command line, `kando`, and an MCP server for AI agents. Run with one of the commands below, it drives the Kando app; run with no command, it opens the window as usual.

To get the `kando` command:

- macOS: move Kando to Applications, then click Install next to Command-Line Tool in Settings › General. It links `kando` to the app in the first of `/usr/local/bin`, `/opt/homebrew/bin` and `~/.local/bin` it can write to, without asking for an administrator password, and never replaces a `kando` of another program. Kando opened from its disk image, or from where macOS translocated it after a download, runs from a path that goes away, so the card asks you to move it first instead of installing a link that would break. Or link it yourself: `sudo ln -s /Applications/Kando.app/Contents/MacOS/Kando /usr/local/bin/kando`.
- Linux: the `.deb` and the `PKGBUILD` install the app in `/opt/kando` and add `/usr/bin/kando`, a link to `/opt/kando/kando`. The install script (see [Download](#download)) installs the app in `~/.local/kando.app` and links `~/.local/bin/kando` to it, leaving alone a `kando` there that it did not make; add `~/.local/bin` to `PATH` if your shell does not have it.
- Windows: Kando cannot install the command, as making a link on Windows needs administrator rights or developer mode. Under Command-Line Tool, Settings › General names the folder that holds `Kando.exe`; add that folder to `PATH`, and `kando` in a terminal runs it, as Windows ignores the case of file names.

| Command | What it does |
| --- | --- |
| `kando status` | Shows the open library, the programs and models in use, and whether a job can start, or what is missing. |
| `kando list [--source S] [query]` | Lists the library's films, newest first, with their ids. `--source` keeps one source (`youtube`, `bilibili`, `local` or another site's name); the query keeps the films whose title contains it. |
| `kando show <film-id>` | Shows one film: its source, stage, and the paths of its video and subtitles. |
| `kando subtitles <film-id> [--format srt\|text\|json] [--original\|--translation]` | Prints a film's subtitles, as SubRip (the default), one line per subtitle (`text`) or JSON, both languages or only one. |
| `kando jobs` | Lists the jobs: the running one, the waiting ones, then the finished ones. |
| `kando add <link-or-file>... [--wait]` | Queues links and local video files, as Start Processing does, and prints each job's id. Relative paths are taken from the current directory. |
| `kando cancel <job-id>` | Stops a running job, or takes a waiting one out of the queue. |
| `kando sites [query] [--limit N]` | Searches the websites Kando downloads from, by name, domain or a video link. |
| `kando open [film-id]` | Brings Kando to the front, showing a film, or the library without an id. |
| `kando mcp` | Serves MCP on standard input and output (see below). |
| `kando version` | Prints the version. |
| `kando help [command]` | Lists the commands, or shows one command's usage and its flags. |

A command that needs the app connects to it through a socket, `kando.sock` in the folder that holds `settings.json` (`~/Library/Application Support/Kando/kando.sock` on macOS, `~/.config/Kando/kando.sock` on Linux, `%AppData%\Kando\kando.sock` on Windows); `KANDO_SOCKET` sets another path, which a second copy of Kando also needs, with its own `HOME` (see [Configuration](#configuration)). When Kando is not running, the command starts it in the background, prints `Starting Kando…` to standard error, and waits up to 30 seconds for it. Adding videos needs what Start Processing needs, such as an open library, the programs and a whisper model; when something is missing, `kando add` fails with the message the window shows for it.

Every command that talks to the app takes `--json`, which prints the app's answer as JSON instead of text. `kando add` exits with 1 when a source could not be added. With `--wait` it stays until every added job is complete or failed, printing a line whenever one's stage or progress changes, and also exits with 1 when any of them failed; with `--json` the progress goes to standard error and the jobs as they ended to standard output. Exit codes are 0 for success, 1 for a failure and 2 for a command used wrongly.

`kando mcp` is an MCP server on standard input and output. Its tools start Kando the same way when it is not running:

| Tool | What it does |
| --- | --- |
| `kando_status` | The app's version, the open library, whether a job can start and what is missing, the programs, the speech and translation models, the target language, and the running and waiting jobs. |
| `kando_list_films` | The open library's films, newest first, filtered by `source` or by words of the title (`query`). |
| `kando_get_film` | One film by its id: its stage, target language, and the paths of its video and subtitles. |
| `kando_get_subtitles` | A finished film's subtitles with timestamps, both languages or one (`which`), as compact text or SRT (`format`), the first `max_cues` lines only when it is set. |
| `kando_add_videos` | Queues links and local video files; returns the jobs, and why each source that was not added was skipped. |
| `kando_list_jobs` | The jobs, with their stage, progress, status, error, and once complete the film id and file paths. |
| `kando_cancel_job` | Stops a running job, or takes a waiting one out of the queue. |
| `kando_search_sites` | Searches the websites Kando downloads from, by name, domain or a video link (20 results unless `limit` is set). |
| `kando_open` | Brings the Kando window to the front, showing a film or the library. |

Add it to Claude Code with the path of the program, `kando` once it is on `PATH`:

```sh
claude mcp add kando -- /Applications/Kando.app/Contents/MacOS/Kando mcp
```

Other MCP clients take the same command in their `mcpServers` setting:

```json
{
  "mcpServers": {
    "kando": {
      "command": "/Applications/Kando.app/Contents/MacOS/Kando",
      "args": ["mcp"]
    }
  }
}
```

The landing page, [kando.yldm.ai](https://kando.yldm.ai), gives browser agents WebMCP tools in browsers that support WebMCP: `search_supported_sites` checks whether Kando downloads from a website, `get_download` gives the download link for a system, `get_started` returns the setup steps, and `list_features` lists what Kando does.

## Configuration

Everything below except the Google client id and secret and `KANDO_SOCKET` can also be set in the app (the library from the sidebar, the rest in Settings). An environment variable, when set, takes the place of the value saved in the app.

| Variable | Default | Meaning |
| --- | --- | --- |
| `KANDO_WHISPER_MODEL` | the model chosen in Settings › Speech Recognition | Path of a whisper.cpp ggml model file, such as `ggml-small.bin`. |
| `KANDO_LLM_BASE_URL` | EveryAPI, `https://api.everyapi.ai/v1` | Base URL of an OpenAI-compatible provider, such as `https://api.openai.com/v1`, or `http://localhost:11434/v1` for Ollama. When it is set, the signed-in EveryAPI account is not used. |
| `KANDO_LLM_MODEL` | `claude-haiku-5-5` on EveryAPI | Translation model. With a custom provider there is no default: set it here or in Settings › AI Service. |
| `KANDO_LLM_API_KEY` | none | Bearer token for the translation provider. It overrides the signed-in EveryAPI account. |
| `EVERYAPI_API_KEY` | none | EveryAPI key, used when `KANDO_LLM_BASE_URL` is unset and `KANDO_LLM_API_KEY` is empty, instead of the CLI login. |
| `KANDO_OUTPUT_DIR` | the library opened last | Library folder to open at launch, where new videos and subtitles go. |
| `KANDO_SOCKET` | `kando.sock` beside `settings.json` | Path of the socket the app listens on and the `kando` command line and `kando mcp` connect to. Set it to the same path for the app and for the commands. Only one copy of Kando runs for a user, and launching another brings the first one forward; a copy started with `KANDO_SOCKET` set is a second copy on purpose, as for development, and needs its own `HOME` too, so that it keeps its own library and settings. See [Command line and MCP](#command-line-and-mcp). |
| `KANDO_GOOGLE_CLIENT_ID` | none | Google OAuth client id for signing in on Settings › Account & Upload. Without it the sign-in button is disabled. It is never saved to the settings file. |
| `KANDO_GOOGLE_CLIENT_SECRET` | none | Optional client secret for the same OAuth client. |
| `KANDO_STORAGE` | `r2` | Where uploads go: `r2`, `s3` (or `aws`), `oss` (or `aliyun`), `cos` (or `tencent`), `compat` (or `s3compat`). Any other value means R2. |
| `KANDO_R2_ENDPOINT` | none | Cloudflare R2: an account id or a full `https://` endpoint. |
| `KANDO_R2_BUCKET`, `KANDO_R2_ACCESS_KEY`, `KANDO_R2_SECRET` | none | Cloudflare R2 bucket and keys. |
| `KANDO_S3_REGION` | none | Amazon S3 region name, such as `ap-northeast-1`. |
| `KANDO_S3_BUCKET`, `KANDO_S3_ACCESS_KEY`, `KANDO_S3_SECRET` | none | Amazon S3 bucket and keys. |
| `KANDO_OSS_REGION` | none | Alibaba Cloud OSS region name, such as `cn-hangzhou` (`cn-hangzhou-internal` for the internal network). |
| `KANDO_OSS_BUCKET`, `KANDO_OSS_ACCESS_KEY`, `KANDO_OSS_SECRET` | none | Alibaba Cloud OSS bucket and keys. |
| `KANDO_COS_REGION` | none | Tencent Cloud COS region name, such as `ap-guangzhou`. |
| `KANDO_COS_BUCKET`, `KANDO_COS_ACCESS_KEY`, `KANDO_COS_SECRET` | none | Tencent Cloud COS bucket (its name includes the APPID, such as `video-1250000000`), SecretId and SecretKey. |
| `KANDO_S3COMPAT_ENDPOINT` | none | Full `https://` endpoint of another S3-compatible store. |
| `KANDO_S3COMPAT_REGION` | none | Signing region of that store; MinIO usually uses `us-east-1`. |
| `KANDO_S3COMPAT_BUCKET`, `KANDO_S3COMPAT_ACCESS_KEY`, `KANDO_S3COMPAT_SECRET` | none | Bucket and keys of that store. |
| `KANDO_S3COMPAT_PATH_STYLE` | off | `1`, `true`, `yes` or `on` turns on path-style addressing, which MinIO needs; `0`, `false`, `no` or `off` turns it off. |

Uploading is manual: open a film and click Upload to R2 (or the store you chose) in the player. Kando uploads the video and its bilingual subtitles under `<Google account email>/<library>/<film folder>/<file>`; the sign-in only says who uploaded. Videos are never uploaded automatically.

## Libraries and settings

A library is a folder whose name ends in `.kando`. Each downloaded film gets its own folder inside it, `<title> [<video id>]/`, holding `video.mp4`, `cover.jpg`, `video.srt` and `video.bilingual.srt`. They are ordinary files: Kando watches the library folder, so videos added or removed in Finder or a file manager show up in the library. On macOS a `.kando` folder gets a folder icon with the Kando mark in Finder.

Removing a film from the library moves the film's folder to the Trash, or only its video and subtitles when that folder holds other videos or folders too; a local video outside the library folder stays where it is and only leaves the list. Removing a library from the list leaves its folder on disk.

Kando keeps its own files in the user config directory, `~/Library/Application Support/Kando` on macOS, `%AppData%\Kando` on Windows and `~/.config/Kando` on Linux:

- `library.json`: the jobs, finished and in progress, and where each video stopped playing.
- `settings.json`: the settings, including the API key, the object storage keys and the Google refresh token, in plain text. The file is readable only by your user.
- `window-state.json`: window sizes and positions.
- `updater.json`: your update choices (the two switches of Settings › General, a version you skipped) and when Kando last checked for updates.
- `models/`: whisper models downloaded in Settings › Speech Recognition.

Cover images of videos that are not named `video.*` are cached in `~/Library/Caches/Kando/posters` on macOS, `%LocalAppData%\Kando\posters` on Windows and `~/.cache/Kando/posters` on Linux.

## Upgrading from LingoCut

Kando was called LingoCut, and nothing reads the old names. To keep your libraries and settings:

1. Quit the app.
2. Rename the config folder from `LingoCut` to `Kando`: `~/Library/Application Support/LingoCut` on macOS, `%AppData%\LingoCut` on Windows, `~/.config/LingoCut` on Linux. It holds `library.json`, `settings.json`, `window-state.json` and the downloaded models.
3. Rename each library from `X.lingocut` to `X.kando`, after renaming the config folder and before launching. On macOS and Linux the saved inodes let the app follow every renamed library. On Windows the app can only match one missing library to one new `.kando` folder in the same parent folder, so rename one library per folder at a time and launch the app once after each rename.
4. Rename each environment variable from `LINGOCUT_*` to `KANDO_*`.
5. Launch Kando and choose the speech model again in Settings › Speech Recognition: the model file moved with the folder and shows as downloaded, but the saved choice still points at the old path.
6. Delete what is left of the old name: `lingocut.app` and `lingocut Dev.app`; the poster cache `~/Library/Caches/LingoCut` (`%LocalAppData%\LingoCut` on Windows, `~/.cache/LingoCut` on Linux); on Linux the lowercase `~/.config/lingocut`; and, if you ran `make dev`, `~/Library/Application Support/lingocut Dev`.

## Development

Requires Go 1.27.3 or later.

```sh
make test   # go test for packages/core, packages/media and apps/kando
make dev    # run the app with live reload
make build  # build the app for this platform into apps/kando/build
```

`make dev` runs MyGo's `mygo dev` patched by `tools/mygo-dev`: on macOS, a reload keeps the running window until the new build shows its own, instead of quitting it first, so saving a change does not look like the app quitting. If the patch stops applying after a MyGo upgrade, it warns and falls back to the stock `go tool mygo dev`; update `tools/mygo-dev/dev.go.patch` against the new `cmd/mygo/dev.go` then. The script behind it, `tools/mygo-dev/dev.sh`, is a POSIX shell script.

The screenshot tests render the main scenes to PNG files from a fixed demo library, in an empty fake home folder with fake `ffmpeg`, `yt-dlp` and `whisper-cli`, so they do not read your settings, libraries or tools:

```sh
KANDO_SCREENSHOTS=/tmp/kando-shots go test ./apps/kando/... -run TestScreenshots
```

`KANDO_SCREENSHOTS_LANG` (`en`, `zh-Hans`, `zh-Hant`, `ja` or `ko`) renders them in that interface language, with a demo library for its reader; `-run TestScreenshotsReadme` renders the four pictures of the READMEs, kept in `docs/assets/<language>/`.

Repository layout:

- `apps/kando`: the app. Each screen has its own directory (`sidebar/`, `films/`, `film/`, `settings/`, `account/`, `cheatsheet/`); `view.go` connects the window to them. `shelf/` is the job record, `cloud/` is Google sign-in and object storage upload, and `brand/` is the Finder icon of a library.
- `packages/core`: platform-independent jobs, stages and link parsing.
- `packages/media`: the media pipeline: download, transcription, translation, subtitle export, covers and libraries.
- `tools/mygo-dev`: the patched `mygo dev` behind `make dev`.

[AGENTS.md](AGENTS.md) describes how the interface is split into components and the rules each area follows; read it before changing the UI.

## CI and releases

The [CI](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml) workflow builds and tests pull requests and `main`. The [Release](https://github.com/yldm-tech/Kando/actions/workflows/release.yml) workflow runs after CI passes on `main`, or by hand: it picks the next version from the Conventional Commits since the last tag, commits it to `main` as `chore(release)`, tags it, builds packages for macOS, Windows and Linux without a Developer ID or code-signing certificate, signs the updates that installed copies of Kando download, and publishes a GitHub release with the macOS and Linux packages and their checksums. The Windows installers are built but not published yet.

## License

[MIT](LICENSE)
