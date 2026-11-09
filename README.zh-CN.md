# 看懂（Kando）

[![CI](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml)
[![Release](https://github.com/yldm-tech/Kando/actions/workflows/release.yml/badge.svg)](https://github.com/yldm-tech/Kando/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/yldm-tech/Kando?include_prereleases&sort=semver)](https://github.com/yldm-tech/Kando/releases)
[![License](https://img.shields.io/github/license/yldm-tech/Kando)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/yldm-tech/Kando?filename=apps%2Fkando%2Fgo.mod)](go.work)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey)](#install)
[![GitHub stars](https://img.shields.io/github/stars/yldm-tech/Kando)](https://github.com/yldm-tech/Kando/stargazers)

[English](README.md) · 简体中文 · [日本語](README.ja.md) · [繁體中文](README.zh-TW.md) · [한국어](README.ko.md)

看懂是一款用原声视频学外语的桌面应用。粘贴 YouTube 或哔哩哔哩链接，或者把视频文件拖到窗口上：看懂会用 yt-dlp 下载视频，在本机用 whisper.cpp 转录语音，再通过 OpenAI 兼容模型逐句翻译，然后播放原视频，屏幕上同时显示原文和译文。它用 Go 编写，原生界面由 [MyGo](https://github.com/egoist/mygo) 和 [Breeze UI](https://github.com/yldm-tech/mygo-breeze-ui) 绘制，没有使用 Web 视图。

![资料库：最近完成的影片、正在处理的任务和资料库中的影片](docs/assets/zh-CN/library.png)

界面支持简体中文、繁体中文、英文、日文和韩文，默认是简体中文；在“设置 › 通用 › 界面语言”中切换，窗口会立即换成所选语言。下文引用的界面文字都与简体中文界面一致。

## 功能

- 添加视频：粘贴 YouTube、哔哩哔哩或 yt-dlp 支持的其他任何网站的链接（“帮助 › 支持的网站…”列出了全部网站及其标志），或者选择本地文件（mp4、mov、mkv、webm 等视频文件），也可以直接拖到窗口上。本地文件在原位置转录，不会被复制。
- 任务按顺序逐个处理，依次经过下载、转录、翻译和导出。失败的任务会显示原因，并给出对应设置项的入口，可以重试或移除。
- 语音识别由 whisper.cpp 在你的电脑上完成，它会检测说话的语言。“设置 › 语音识别”列出七个 whisper 模型，从 Tiny（77.7 MB）到 Large v3（3.1 GB），你选中的模型会从 Hugging Face 下载。
- 台词默认由 EveryAPI 翻译，也可以使用任何 OpenAI 兼容的聊天 API：“设置 › AI 服务”提供 OpenAI、OpenRouter、Groq、DeepSeek、xAI、Mistral、通义千问（DashScope）、Kimi、智谱 GLM、硅基流动、Ollama 和 LM Studio 的预设，还可以自定义接口地址。目标语言、“智能翻译”（让模型参考前面几句台词）和提示词，都可以在“设置 › 翻译”中修改。
- 每个处理完成的视频旁边会生成两个 SubRip 字幕文件：`<video>.srt` 是原文字幕，`<video>.bilingual.srt` 中每句译文位于对应原文上方（下载的视频则是 `video.srt` 和 `video.bilingual.srt`）。
- 播放器可以把字幕显示为双语、仅原文、仅译文或关闭，字幕延迟可按 0.1 秒步进调整，样式也可以修改。侧边面板列出全部台词并支持搜索（单击某句即可跳转），把视频切分成 4 到 12 个约 30 秒的片段，并显示影片的文件和来源。每部影片都会从你上次停下的位置继续播放。
- 资料库是 `.kando` 文件夹，里面都是普通文件，可以在访达或文件管理器中管理。可以同时保留多个资料库，并在侧栏中切换；侧栏还能按来源筛选当前资料库中的影片：YouTube、哔哩哔哩、其他网站和本地文件。
- 可选：登录 Google 后，在播放器中把影片的视频和双语字幕上传到你自己的对象存储（Cloudflare R2、Amazon S3、阿里云 OSS、腾讯云 COS 或任何 S3 兼容存储）。参见[配置](#configuration)。
- 播放、字幕和窗口都有键盘快捷键，可以在“快捷键速查”（⌘/）和“设置 › 快捷键”中查看。

![带双语字幕的播放器，以及侧边面板中的台词列表](docs/assets/zh-CN/player.png)

<a id="install"></a>

## 安装

### 下载

版本发布后，macOS、Windows 和 Linux 的安装包会附在 [GitHub Releases](https://github.com/yldm-tech/Kando/releases) 上。这些安装包没有用 Developer ID 或代码签名证书签名，因此第一次打开看懂时系统会发出警告：

- macOS：在 macOS 15 及更高版本上，先打开一次看懂，然后选择“系统设置 › 隐私与安全性 › 仍要打开”。在 macOS 14 及更早版本上，在“应用程序”文件夹中按住 Control 键点按看懂，然后选择“打开”。也可以在终端中移除隔离标记：`xattr -dr com.apple.quarantine /Applications/Kando.app`。
- Windows：在 SmartScreen 对话框中选择“更多信息 › 仍要运行”。

### 从源码构建

需要 Go 1.27.3 或更高版本：

```sh
git clone https://github.com/yldm-tech/Kando.git
cd Kando
make build
```

`make build` 会针对当前平台运行 `go tool mygo build apps/kando`，并把结果写入 `apps/kando/build/<os>-<arch>/`：在 macOS 上是经过临时签名（ad-hoc）的 `Kando.app` 和一个 `.dmg`，在其他平台上是一个可执行文件（Linux 另外生成一个 `.deb`、一个附带安装脚本的 `.tar.gz` 和一个 `PKGBUILD`；Windows 另外生成一个用 NSIS 制作的安装程序，未安装 NSIS 时 mygo 会自动下载）。没有 `make` 时（例如未额外安装工具的 Windows），在仓库根目录直接运行 `go tool mygo build apps/kando`。

## 运行依赖

看懂会调用以下程序，但不负责安装或更新它们。

| 程序 | 用途 |
| --- | --- |
| `yt-dlp` | 下载在线视频。每个任务都需要它，本地文件也不例外。 |
| `ffmpeg` | 提取音频、截取封面，以及在播放器中解码画面。在 macOS 上，声音由随 ffmpeg 一起安装的 `ffplay` 或系统自带的 `afplay` 播放。 |
| `whisper-cli`（whisper.cpp） | 语音识别。 |
| 一个 whisper.cpp ggml 模型 | 在“设置 › 语音识别”中下载，或者用 `KANDO_WHISPER_MODEL` 指向一个模型文件。 |

在 macOS 上可以用 Homebrew 安装：

```sh
brew install yt-dlp ffmpeg whisper-cpp
```

在 Windows 和 Linux 上，用包管理器安装同样的程序，并确保它们在 `PATH` 中。除了 `PATH`，看懂还会查找 `/opt/homebrew/bin`、`/usr/local/bin` 和 `/home/linuxbrew/.linuxbrew/bin`，因为从访达或桌面启动器打开的应用只能拿到精简的 `PATH`。“设置 › 通用”会显示找到了哪些程序，点击“重新检测”可以重新查找。

在 Windows 和 Linux 上，播放器目前只有画面没有声音；音频播放只在 macOS 上实现。

## 快速上手

1. 打开看懂。还没有资料库时，它会提供“新建资料库…”和“打开已有资料库…”。新建的资料库默认名为“影视库”，在 macOS 上位于 `~/Movies`，在其他系统上位于 `~/Videos`，可以自行修改。
2. 打开设置（⌘,）。在“通用”中确认 `ffmpeg`、`yt-dlp` 和 `whisper-cli` 都显示为“已安装”。
3. 在“语音识别”中，点击某个 whisper 模型旁的“下载”；下载完成后看懂会自动选用它。标为“推荐”的是 Large v3 Turbo（压缩版），大小 574 MB。Vimpo 或 hyperframes 已经下载过的模型可以直接选用，无需复制。
4. 在“AI 服务”中设置翻译：
   - EveryAPI（默认）：运行一次 `everyapi auth login`，用 [EveryAPI](https://everyapi.ai) CLI 登录。看懂会向 CLI 获取密钥（`everyapi auth credential`），因此登录状态、网关区域和密钥刷新都与 CLI 保持一致，无需手动粘贴密钥。默认模型是 `claude-haiku-5-5`，其他模型可以用 `everyapi models list` 查看。也可以直接把 EveryAPI 密钥粘贴到“API Key”中。
   - 其他服务商：在“服务”中选择它，或者选择“OpenAI 兼容”并填写它的 Base URL，然后填写“模型”和“API Key”（Ollama、LM Studio 等本地服务不需要密钥）。“测试连接”会发送一个简短的请求来检查设置。
5. 在“翻译”中选择默认目标语言：简体中文（默认）、繁体中文、English、日本語、한국어、Español、Français 或 Deutsch。
6. 回到主窗口，把链接粘贴到顶栏的输入框并点击“开始处理”，或者点击“本地视频”，也可以把视频拖到窗口上。任务所需的一切都就绪时，侧栏底部的状态行会显示“已就绪，可以开始处理”，否则会指出缺少什么。

遇到翻译服务商限流或返回服务器错误时，看懂会自动重新发送请求，连同第一次最多尝试三次。

![设置 › 通用：当前打开的资料库、界面语言、版本以及看懂找到的程序](docs/assets/zh-CN/settings-general.png)

![设置 › AI 服务，已选择 EveryAPI](docs/assets/zh-CN/settings-ai.png)

<a id="configuration"></a>

## 配置

除 Google 客户端 ID 和客户端密钥外，下表各项也都可以在应用内设置（资料库在侧栏中切换，其他项在“设置”中修改）。环境变量一旦设置，就会覆盖应用中保存的值。

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `KANDO_WHISPER_MODEL` | 在“设置 › 语音识别”中选择的模型 | whisper.cpp ggml 模型文件的路径，例如 `ggml-small.bin`。 |
| `KANDO_LLM_BASE_URL` | EveryAPI，`https://api.everyapi.ai/v1` | OpenAI 兼容服务商的 Base URL，例如 `https://api.openai.com/v1`，Ollama 则是 `http://localhost:11434/v1`。设置后不再使用已登录的 EveryAPI 账号。 |
| `KANDO_LLM_MODEL` | EveryAPI 上为 `claude-haiku-5-5` | 翻译模型。使用自定义服务商时没有默认值：需要在这里或“设置 › AI 服务”中设置。 |
| `KANDO_LLM_API_KEY` | 无 | 翻译服务商的 API 密钥（以 Bearer 方式发送）。它优先于已登录的 EveryAPI 账号。 |
| `EVERYAPI_API_KEY` | 无 | EveryAPI 密钥，在 `KANDO_LLM_BASE_URL` 未设置且 `KANDO_LLM_API_KEY` 为空时使用，代替 CLI 登录。 |
| `KANDO_OUTPUT_DIR` | 上次打开的资料库 | 启动时打开的资料库文件夹，新的视频和字幕都存放在这里。 |
| `KANDO_GOOGLE_CLIENT_ID` | 无 | 用于在“设置 › 账号与上传”中登录的 Google OAuth 客户端 ID。不设置时登录按钮不可用。它永远不会保存到设置文件中。 |
| `KANDO_GOOGLE_CLIENT_SECRET` | 无 | 同一 OAuth 客户端的客户端密钥，可选。 |
| `KANDO_STORAGE` | `r2` | 上传目标：`r2`、`s3`（或 `aws`）、`oss`（或 `aliyun`）、`cos`（或 `tencent`）、`compat`（或 `s3compat`）。其他任何值都按 R2 处理。 |
| `KANDO_R2_ENDPOINT` | 无 | Cloudflare R2：账号 ID 或完整的 `https://` 端点。 |
| `KANDO_R2_BUCKET`、`KANDO_R2_ACCESS_KEY`、`KANDO_R2_SECRET` | 无 | Cloudflare R2 的存储桶和密钥。 |
| `KANDO_S3_REGION` | 无 | Amazon S3 区域名称，例如 `ap-northeast-1`。 |
| `KANDO_S3_BUCKET`、`KANDO_S3_ACCESS_KEY`、`KANDO_S3_SECRET` | 无 | Amazon S3 的存储桶和密钥。 |
| `KANDO_OSS_REGION` | 无 | 阿里云 OSS 地域名称，例如 `cn-hangzhou`（内网使用 `cn-hangzhou-internal`）。 |
| `KANDO_OSS_BUCKET`、`KANDO_OSS_ACCESS_KEY`、`KANDO_OSS_SECRET` | 无 | 阿里云 OSS 的存储桶和密钥。 |
| `KANDO_COS_REGION` | 无 | 腾讯云 COS 地域名称，例如 `ap-guangzhou`。 |
| `KANDO_COS_BUCKET`、`KANDO_COS_ACCESS_KEY`、`KANDO_COS_SECRET` | 无 | 腾讯云 COS 的存储桶（名称包含 APPID，例如 `video-1250000000`）、SecretId 和 SecretKey。 |
| `KANDO_S3COMPAT_ENDPOINT` | 无 | 其他 S3 兼容存储的完整 `https://` 端点。 |
| `KANDO_S3COMPAT_REGION` | 无 | 该存储的签名区域；MinIO 通常使用 `us-east-1`。 |
| `KANDO_S3COMPAT_BUCKET`、`KANDO_S3COMPAT_ACCESS_KEY`、`KANDO_S3COMPAT_SECRET` | 无 | 该存储的存储桶和密钥。 |
| `KANDO_S3COMPAT_PATH_STYLE` | 关闭 | `1`、`true`、`yes` 或 `on` 开启路径样式寻址（MinIO 需要）；`0`、`false`、`no` 或 `off` 关闭。 |

上传需要手动进行：打开一部影片，在播放器中点击“上传到 R2”（或你选择的存储）。看懂会把视频及其双语字幕上传到 `<Google 账号邮箱>/<资料库>/<影片文件夹>/<文件>` 下；登录只用于标明是谁上传的。视频永远不会自动上传。

## 资料库与设置

资料库是名称以 `.kando` 结尾的文件夹。每部下载的影片在其中都有自己的文件夹 `<title> [<video id>]/`，里面是 `video.mp4`、`cover.jpg`、`video.srt` 和 `video.bilingual.srt`。它们都是普通文件：看懂会监视资料库文件夹，所以在访达或文件管理器中添加或删除的视频都会反映到资料库中。在 macOS 上，`.kando` 文件夹在访达中会显示为带有看懂标志的文件夹图标。

从资料库移除影片时，会把影片的文件夹移到废纸篓；如果该文件夹里还有其他视频或文件夹，则只移走这部影片的视频和字幕。资料库文件夹之外的本地视频会留在原处，只是从列表中消失。从列表中移除资料库时，它在磁盘上的文件夹会保留。

看懂把自己的文件放在用户配置目录中（macOS 上是 `~/Library/Application Support/Kando`，Windows 上是 `%AppData%\Kando`，Linux 上是 `~/.config/Kando`），包括：

- `library.json`：所有任务（已完成的和进行中的），以及每个视频上次播放到的位置。
- `settings.json`：设置，包括 API 密钥、对象存储密钥和 Google 刷新令牌，均以明文保存。该文件仅当前用户可读。
- `window-state.json`：窗口大小和位置。
- `models/`：在“设置 › 语音识别”中下载的 whisper 模型。

文件名不是 `video.*` 的视频，其封面图缓存在 `~/Library/Caches/Kando/posters`（macOS）、`%LocalAppData%\Kando\posters`（Windows）和 `~/.cache/Kando/posters`（Linux）中。

## 从 LingoCut 升级

看懂以前叫 LingoCut，新版本不会再读取任何使用旧名称的路径或变量。要保留原有的资料库和设置，请按以下步骤操作：

1. 退出应用。
2. 把配置文件夹从 `LingoCut` 重命名为 `Kando`：macOS 上是 `~/Library/Application Support/LingoCut`，Windows 上是 `%AppData%\LingoCut`，Linux 上是 `~/.config/LingoCut`。其中包含 `library.json`、`settings.json`、`window-state.json` 和已下载的模型。
3. 在重命名配置文件夹之后、启动应用之前，把每个资料库从 `X.lingocut` 重命名为 `X.kando`。在 macOS 和 Linux 上，应用可以通过保存的文件索引节点（inode）找到每个改名后的资料库。在 Windows 上，应用只能把一个找不到的资料库匹配到同一父文件夹中的一个新 `.kando` 文件夹，所以同一个父文件夹中一次只重命名一个资料库，每改一个就启动一次应用。
4. 把每个环境变量从 `LINGOCUT_*` 重命名为 `KANDO_*`。
5. 启动看懂，并在“设置 › 语音识别”中重新选择 whisper 模型：模型文件已随文件夹一起移动，会显示为“已下载”，但保存的选择仍指向旧路径。
6. 删除旧名称留下的内容：`lingocut.app` 和 `lingocut Dev.app`；封面缓存 `~/Library/Caches/LingoCut`（Windows 上是 `%LocalAppData%\LingoCut`，Linux 上是 `~/.cache/LingoCut`）；Linux 上的小写目录 `~/.config/lingocut`；如果运行过 `make dev`，还有 `~/Library/Application Support/lingocut Dev`。

## 开发

需要 Go 1.27.3 或更高版本。

```sh
make test   # 运行 packages/core、packages/media 和 apps/kando 的 go test
make dev    # 以热重载方式运行应用
make build  # 为当前平台构建应用，输出到 apps/kando/build
```

`make dev` 运行的是经 `tools/mygo-dev` 打过补丁的 MyGo `mygo dev`：在 macOS 上，重新加载时会保留正在运行的窗口，等新构建的窗口出现后再替换，而不是先退出旧窗口，所以保存改动时不会看起来像应用退出了。如果 MyGo 升级后补丁无法应用，它会发出警告并回退到原版的 `go tool mygo dev`；这时请对照新的 `cmd/mygo/dev.go` 更新 `tools/mygo-dev/dev.go.patch`。它背后的 `tools/mygo-dev/dev.sh` 是一个 POSIX 兼容的 shell 脚本。

截图测试会从一个固定的演示资料库把主要场景渲染成 PNG 文件，运行时使用空的模拟主目录以及模拟的 `ffmpeg`、`yt-dlp` 和 `whisper-cli`，因此不会读取你的设置、资料库或工具：

```sh
KANDO_SCREENSHOTS=/tmp/kando-shots go test ./apps/kando/... -run TestScreenshots
```

设置 `KANDO_SCREENSHOTS_LANG`（`en`、`zh-Hans`、`zh-Hant`、`ja` 或 `ko`）会以对应的界面语言渲染，演示资料库也换成该语言读者的内容；`-run TestScreenshotsReadme` 渲染各语言 README 用的四张图，保存在 `docs/assets/<语言>/`。

仓库结构：

- `apps/kando`：应用本体。每个界面有自己的目录（`sidebar/`、`films/`、`film/`、`settings/`、`account/`、`cheatsheet/`）；`view.go` 把窗口和它们连接起来。`shelf/` 是任务记录，`cloud/` 负责 Google 登录和对象存储上传，`brand/` 是资料库在访达中的图标。
- `packages/core`：与平台无关的任务、阶段和链接解析。
- `packages/media`：媒体处理流水线，包括下载、转录、翻译、字幕导出、封面和资料库。
- `tools/mygo-dev`：`make dev` 背后打过补丁的 `mygo dev`。

[AGENTS.md](AGENTS.md) 说明了界面如何拆分成组件，以及各部分遵循的规则；修改界面前请先阅读。

## CI 与发布

[CI](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml) 工作流会对拉取请求和 `main` 分支进行构建和测试。[Release](https://github.com/yldm-tech/Kando/actions/workflows/release.yml) 工作流在 `main` 分支的 CI 通过后运行，也可以手动触发：它根据上一个标签以来的 约定式提交（Conventional Commits）确定下一个版本号，以 `chore(release)` 提交到 `main`，打上标签，在不使用 Developer ID 或代码签名证书的情况下构建 macOS、Windows 和 Linux 的安装包，并连同校验和一起发布到 GitHub Releases。

## 许可证

[MIT](LICENSE)
