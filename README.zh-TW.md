# 看懂（Kando）

[![CI](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml)
[![Release](https://github.com/yldm-tech/Kando/actions/workflows/release.yml/badge.svg)](https://github.com/yldm-tech/Kando/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/yldm-tech/Kando?include_prereleases&sort=semver)](https://github.com/yldm-tech/Kando/releases)
[![License](https://img.shields.io/github/license/yldm-tech/Kando)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/yldm-tech/Kando?filename=apps%2Fkando%2Fgo.mod)](go.work)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey)](#install)
[![GitHub stars](https://img.shields.io/github/stars/yldm-tech/Kando)](https://github.com/yldm-tech/Kando/stargazers)

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · 繁體中文 · [한국어](README.ko.md)

看懂是一款桌面應用程式，讓你透過某種語言的影片來學習那種語言。貼上 YouTube 或嗶哩嗶哩的連結，或把影片檔拖曳到視窗上：看懂會用 yt-dlp 下載影片，用 whisper.cpp 在你自己的電腦上轉錄語音，透過 OpenAI 相容的模型翻譯每一句台詞，再播放原始影片，並在畫面上同時顯示原文台詞與譯文。它以 Go 撰寫，使用 [MyGo](https://github.com/egoist/mygo) 和 [Breeze UI](https://github.com/yldm-tech/mygo-breeze-ui) 繪製原生介面，沒有使用網頁檢視（web view）。

![資料庫：最近完成的影片、處理中的工作，以及資料庫中的影片](docs/assets/zh-TW/library.png)

介面支援繁體中文、簡體中文、英文、日文和韓文，預設為簡體中文；在「設定 › 一般 › 介面語言」中切換，視窗會立即改用所選語言。下文引用的標籤都與繁體中文介面一致。

## 功能

- 新增影片的方式：貼上 YouTube、嗶哩嗶哩或其他 yt-dlp 支援之網站的連結（「輔助說明 › 支援的網站…」列出全部網站及其標誌），或選擇本機檔案（mp4、mov、mkv、webm 及其他影片檔），也可以把檔案拖曳到視窗上。本機檔案會在原處轉錄，不會被複製。
- 工作會依序經過下載、轉錄、翻譯和匯出各階段。失敗的工作會說明原因，附上能修正問題的設定項目連結，並可重試或移除。
- 語音辨識在你的電腦上以 whisper.cpp 進行，whisper.cpp 會自動偵測影片所說的語言。「設定 › 語音辨識」列出七個 whisper 模型，從 Tiny（77.7 MB）到 Large v3（3.1 GB），並從 Hugging Face 下載你選擇的那一個。
- 台詞預設由 EveryAPI 翻譯，也可以使用任何 OpenAI 相容的聊天 API：「設定 › AI 服務」內建 OpenAI、OpenRouter、Groq、DeepSeek、xAI、Mistral、通义千问（Qwen／DashScope）、Kimi、智谱 GLM（Zhipu）、硅基流动（SiliconFlow）、Ollama 和 LM Studio 的預設選項，另可自訂端點。目標語言、讓模型參考前面幾句台詞的情境感知模式，以及提示詞，都可以在「設定 › 翻譯」中修改。
- 每部處理完成的影片旁邊會產生兩個 SubRip 字幕檔：`<video>.srt` 是原文台詞，`<video>.bilingual.srt` 則把每句譯文放在原文台詞上方（下載的影片則是 `video.srt` 和 `video.bilingual.srt`）。
- 播放器可將字幕顯示為雙語、僅原文、僅譯文或關閉，延遲可以 0.1 秒為單位調整，樣式也可以變更。側邊面板列出所有台詞並可搜尋（按一下台詞即可跳到該處），把影片切成 4 到 12 個約 30 秒的片段，並顯示影片的檔案與來源。每部影片都會從你上次停止觀看的地方繼續播放。
- 資料庫是由一般檔案組成的 `.kando` 資料夾，可以在 Finder 或檔案管理員中管理。你可以保留多個資料庫，並從側邊欄切換；側邊欄也能依來源篩選資料庫：YouTube、嗶哩嗶哩、其他網站和本機檔案。
- 可選擇在使用 Google 登入後，從播放器把影片檔與雙語字幕檔上傳到你自己的物件儲存空間（Cloudflare R2、Amazon S3、阿里雲 OSS、騰訊雲 COS，或任何 S3 相容的儲存服務）。請參閱[組態設定](#configuration)。
- 播放、字幕與視窗都有鍵盤快速鍵，列在速查表（⌘/）和「設定 › 快速鍵」中。

![顯示雙語字幕的播放器，以及側邊面板中的台詞清單](docs/assets/zh-TW/player.png)

<a id="install"></a>

## 安裝

### 下載

macOS（Apple 晶片和 Intel）和 Linux（x86-64 和 ARM64）的建置版本在 [GitHub Releases](https://github.com/yldm-tech/Kando/releases) 上；目前還沒有 Windows 版本，在 Windows 上請依下文從原始碼建置。這些建置版本沒有以 Developer ID 或程式碼簽署憑證簽署，因此第一次開啟看懂時，系統會發出警告：

- macOS：在 macOS 15 及更新版本上，先開啟看懂一次，再選擇「系統設定 › 隱私權與安全性 › 強制打開」。在 macOS 14 及更早版本上，在「應用程式」中按住 Control 鍵並按一下 Kando，然後選擇「打開」。也可以在「終端機」中移除隔離標記：`xattr -dr com.apple.quarantine /Applications/Kando.app`。
- Windows：在 SmartScreen 對話方塊中，選擇「其他資訊 › 仍要執行」。

### 從原始碼建置

需要 Go 1.27.3 或更新版本：

```sh
git clone https://github.com/yldm-tech/Kando.git
cd Kando
make build
```

`make build` 會針對你目前所在的平台執行 `go tool mygo build apps/kando`，並把結果寫到 `apps/kando/build/<os>-<arch>/` 底下：在 macOS 上是以 ad-hoc 方式簽署的 `Kando.app` 和一個 `.dmg`，在其他平台上是執行檔（Linux 另外還有 `.deb`、附安裝腳本的 `.tar.gz` 和 `PKGBUILD`；Windows 則另有以 NSIS 製作的安裝程式，未安裝 NSIS 時 mygo 會自行下載）。沒有 `make` 時（例如預設的 Windows），請在存放庫根目錄直接執行 `go tool mygo build apps/kando`。

## 系統需求

看懂會執行 `yt-dlp`、`ffmpeg` 和 `whisper-cli`（whisper.cpp）。在 macOS 和 Linux 上不必自己安裝：看懂開啟時若缺少其中任何一個，會自動下載，入門導覽中會顯示下載進度。已經安裝好的（例如用 Homebrew 安裝的）會直接使用。

| 程式 | 用途 | 看懂下載的版本 |
| --- | --- | --- |
| `yt-dlp` | 下載線上影片。每個工作都需要它，本機檔案也不例外。 | [yt-dlp 官方發佈](https://github.com/yt-dlp/yt-dlp/releases)的單一檔案版。 |
| `ffmpeg` | 擷取音訊、產生封面，以及在播放器中解碼畫面。在 macOS 上，聲音透過它旁邊的 `ffplay` 播放，或改用 `afplay`。 | FFmpeg 9.0.2（macOS 版附 `ffplay`），來自本儲存庫的 [tools 發佈](https://github.com/yldm-tech/Kando/releases/tag/tools-1)。 |
| `whisper-cli`（whisper.cpp） | 語音辨識。 | whisper.cpp 1.9.5（macOS 版啟用 Metal），來自同一個發佈。 |
| 一個 whisper.cpp ggml 模型 | 在入門導覽或「設定 › 語音辨識」中下載一個，或把 `KANDO_WHISPER_MODEL` 指向一個模型檔。 | |

tools 發佈由 Tools 工作流程（`.github/workflows/tools.yml`）依 `.github/tools.env` 中固定的原始碼建置，看懂使用每個下載前都會以內建的 SHA-256 驗證。下載的程式放在 `settings.json` 旁的 `tools` 資料夾（macOS 上是 `~/Library/Application Support/Kando/tools`，Linux 上是 `~/.config/Kando/tools`）。看懂會先在這裡找，再找 `PATH` 以及 `/opt/homebrew/bin`、`/usr/local/bin` 和 `/home/linuxbrew/.linuxbrew/bin`，因為從 Finder 或桌面啟動器開啟的應用程式只會拿到精簡的 `PATH`。「設定 › 一般」會顯示使用的是哪個程式，缺少的可以按「下載」或「重試」，按「重新偵測」會再找一次。若因 `yt-dlp` 版本過舊而下載失敗，看懂會提供「更新並重試」，依原本的安裝方式更新它並重試。

在 Windows 上看懂暫時還不會自動下載：請用你的套件管理器安裝相同的程式，並確認它們位於 `PATH` 中。

在 Windows 和 Linux 上，播放器目前只顯示畫面而沒有聲音；音訊播放只在 macOS 上實作。

## 開始使用

1. 開啟看懂。還沒有任何資料庫時，它會提供「新增資料庫…」和「打開現有資料庫…」兩個選項。新資料庫預設命名為「影視庫」，在 macOS 上位於 `~/Movies`，其他平台位於 `~/Videos`，可以自行更改。
2. 入門導覽會自動下載缺少的 `ffmpeg`、`yt-dlp` 和 `whisper-cli`。開啟「設定」（⌘,），「一般」中會顯示它們和各自的版本。
3. 在「語音辨識」中，按一下某個 whisper 模型旁的「下載」；下載完成後，看懂會選用它。標示「推薦」的是「Large v3 Turbo（壓縮版）」，574 MB。Vimpo 或 hyperframes 已經下載的模型可以直接選用，不必複製。
4. 在「AI 服務」中設定翻譯：
   - EveryAPI（預設）：執行 `everyapi auth login`，用 [EveryAPI](https://everyapi.ai) CLI 登入一次。看懂會向 CLI 取得金鑰（`everyapi auth credential`），因此會沿用 CLI 的登入狀態、閘道區域與金鑰更新，你不必貼上權杖。預設模型是 `claude-haiku-5-5`；`everyapi models list` 會列出其他模型。你也可以改為把 EveryAPI 金鑰貼到「API Key」中。
   - 其他供應商：在「服務」中選擇它，或選擇「OpenAI 相容」並輸入它的「Base URL」，然後填寫「模型」和「API Key」。「測試連線」會送出一個簡短的請求來檢查設定。
5. 在「翻譯」中選擇預設目標語言：簡體中文（預設）、繁體中文、English、日本語、한국어、Español、Français 或 Deutsch。
6. 回到主視窗，把連結貼到頂端列並按一下「開始處理」，或按一下「本機影片」，或把影片拖曳到視窗上。當工作所需的一切都就緒時，側邊欄底部的狀態列會顯示「已就緒，可以開始處理」，否則會指出缺少什麼。

翻譯供應商回傳速率限制或伺服器錯誤時，看懂會重新送出請求，連同第一次最多嘗試三次。

![「設定 › 一般」：目前開啟的資料庫、介面語言、版本，以及看懂找到的程式](docs/assets/zh-TW/settings-general.png)

![已選擇 EveryAPI 的「設定 › AI 服務」](docs/assets/zh-TW/settings-ai.png)

<a id="configuration"></a>

## 組態設定

下列所有項目，除了 Google 用戶端 ID 和用戶端密鑰之外，也都可以在應用程式中設定（資料庫在側邊欄設定，其餘在「設定」中）。環境變數一旦設定，就會取代應用程式中儲存的值。

| 變數 | 預設值 | 說明 |
| --- | --- | --- |
| `KANDO_WHISPER_MODEL` | 在「設定 › 語音辨識」中選擇的模型 | whisper.cpp ggml 模型檔的路徑，例如 `ggml-small.bin`。 |
| `KANDO_LLM_BASE_URL` | EveryAPI，`https://api.everyapi.ai/v1` | OpenAI 相容供應商的 Base URL，例如 `https://api.openai.com/v1`，或 Ollama 的 `http://localhost:11434/v1`。設定後，就不會使用已登入的 EveryAPI 帳號。 |
| `KANDO_LLM_MODEL` | 在 EveryAPI 上為 `claude-haiku-5-5` | 翻譯模型。使用自訂供應商時沒有預設值：請在這裡或「設定 › AI 服務」中設定。 |
| `KANDO_LLM_API_KEY` | 無 | 翻譯供應商的 Bearer 權杖。它會覆寫已登入的 EveryAPI 帳號。 |
| `EVERYAPI_API_KEY` | 無 | EveryAPI 金鑰，在 `KANDO_LLM_BASE_URL` 未設定且 `KANDO_LLM_API_KEY` 為空時使用，取代 CLI 登入。 |
| `KANDO_OUTPUT_DIR` | 上次開啟的資料庫 | 啟動時要開啟的資料庫資料夾，新的影片和字幕都會放在這裡。 |
| `KANDO_GOOGLE_CLIENT_ID` | 無 | 用於在「設定 › 帳號與上傳」登入的 Google OAuth 用戶端 ID。沒有設定時，登入按鈕會停用。它永遠不會儲存到設定檔中。 |
| `KANDO_GOOGLE_CLIENT_SECRET` | 無 | 選填，同一個 OAuth 用戶端的用戶端密鑰。 |
| `KANDO_STORAGE` | `r2` | 上傳的目的地：`r2`、`s3`（或 `aws`）、`oss`（或 `aliyun`）、`cos`（或 `tencent`）、`compat`（或 `s3compat`）。其他任何值都視為 R2。 |
| `KANDO_R2_ENDPOINT` | 無 | Cloudflare R2：帳號 ID 或完整的 `https://` 端點。 |
| `KANDO_R2_BUCKET`、`KANDO_R2_ACCESS_KEY`、`KANDO_R2_SECRET` | 無 | Cloudflare R2 的儲存貯體和金鑰。 |
| `KANDO_S3_REGION` | 無 | Amazon S3 區域名稱，例如 `ap-northeast-1`。 |
| `KANDO_S3_BUCKET`、`KANDO_S3_ACCESS_KEY`、`KANDO_S3_SECRET` | 無 | Amazon S3 的儲存貯體和金鑰。 |
| `KANDO_OSS_REGION` | 無 | 阿里雲 OSS 區域名稱，例如 `cn-hangzhou`（內部網路使用 `cn-hangzhou-internal`）。 |
| `KANDO_OSS_BUCKET`、`KANDO_OSS_ACCESS_KEY`、`KANDO_OSS_SECRET` | 無 | 阿里雲 OSS 的儲存貯體和金鑰。 |
| `KANDO_COS_REGION` | 無 | 騰訊雲 COS 區域名稱，例如 `ap-guangzhou`。 |
| `KANDO_COS_BUCKET`、`KANDO_COS_ACCESS_KEY`、`KANDO_COS_SECRET` | 無 | 騰訊雲 COS 的儲存貯體（名稱包含 APPID，例如 `video-1250000000`）、SecretId 和 SecretKey。 |
| `KANDO_S3COMPAT_ENDPOINT` | 無 | 其他 S3 相容儲存服務的完整 `https://` 端點。 |
| `KANDO_S3COMPAT_REGION` | 無 | 該儲存服務的簽章區域；MinIO 通常使用 `us-east-1`。 |
| `KANDO_S3COMPAT_BUCKET`、`KANDO_S3COMPAT_ACCESS_KEY`、`KANDO_S3COMPAT_SECRET` | 無 | 該儲存服務的儲存貯體和金鑰。 |
| `KANDO_S3COMPAT_PATH_STYLE` | 關閉 | `1`、`true`、`yes` 或 `on` 會開啟 MinIO 所需的路徑樣式定址；`0`、`false`、`no` 或 `off` 會關閉它。 |

上傳需要手動進行：開啟一部影片，在播放器中按一下「上傳到 R2」，或你所選儲存服務對應的按鈕。看懂會把影片和它的雙語字幕上傳到 `<Google 帳號電子郵件>/<資料庫>/<影片資料夾>/<檔案>` 底下；登入只用來表明是誰上傳的。影片永遠不會自動上傳。

## 資料庫與設定

資料庫是名稱以 `.kando` 結尾的資料夾。每部下載的影片在其中都有自己的資料夾 `<title> [<video id>]/`，內含 `video.mp4`、`cover.jpg`、`video.srt` 和 `video.bilingual.srt`。它們都是一般檔案：看懂會監看資料庫資料夾，因此在 Finder 或檔案管理員中新增或移除的影片都會反映在資料庫中。在 macOS 上，`.kando` 資料夾在 Finder 中會顯示帶有看懂標誌的資料夾圖示。

從資料庫移除影片時，會把該影片的資料夾移到垃圾桶；如果那個資料夾裡還有其他影片或資料夾，則只移走它的影片檔和字幕檔。位於資料庫資料夾之外的本機影片會留在原處，只會從清單中移除。從清單中移除資料庫時，它的資料夾會保留在磁碟上。

看懂把自己的檔案放在使用者設定目錄中，macOS 是 `~/Library/Application Support/Kando`，Windows 是 `%AppData%\Kando`，Linux 是 `~/.config/Kando`：

- `library.json`：工作（包括已完成和進行中的），以及每部影片停止播放的位置。
- `settings.json`：各項設定，包括 API 金鑰、物件儲存金鑰和 Google 更新權杖，皆以純文字儲存。這個檔案只有你的使用者帳號可以讀取。
- `window-state.json`：視窗大小和位置。
- `models/`：在「設定 › 語音辨識」中下載的 whisper 模型。

檔名不是 `video.*` 的影片，其封面圖片會快取在 macOS 的 `~/Library/Caches/Kando/posters`、Windows 的 `%LocalAppData%\Kando\posters` 和 Linux 的 `~/.cache/Kando/posters`。

## 從 LingoCut 升級

看懂以前叫做 LingoCut，新版不會再讀取任何舊名稱。若要保留你的資料庫和設定：

1. 結束應用程式。
2. 把設定資料夾從 `LingoCut` 重新命名為 `Kando`：macOS 是 `~/Library/Application Support/LingoCut`，Windows 是 `%AppData%\LingoCut`，Linux 是 `~/.config/LingoCut`。其中存放著 `library.json`、`settings.json`、`window-state.json` 和已下載的模型。
3. 在重新命名設定資料夾之後、啟動應用程式之前，把每個資料庫從 `X.lingocut` 重新命名為 `X.kando`。在 macOS 和 Linux 上，應用程式能透過儲存的 inode 追蹤每個重新命名過的資料庫。在 Windows 上，應用程式只能把一個找不到的資料庫對應到同一個上層資料夾中的一個新 `.kando` 資料夾，所以每個資料夾一次只重新命名一個資料庫，並在每次重新命名後啟動應用程式一次。
4. 把每個環境變數從 `LINGOCUT_*` 重新命名為 `KANDO_*`。
5. 啟動看懂，並在「設定 › 語音辨識」中重新選擇語音模型：模型檔已隨資料夾一起搬移並顯示為「已下載」，但儲存的選擇仍指向舊路徑。
6. 刪除舊名稱留下的東西：`lingocut.app` 和 `lingocut Dev.app`；封面快取 `~/Library/Caches/LingoCut`（Windows 是 `%LocalAppData%\LingoCut`，Linux 是 `~/.cache/LingoCut`）；Linux 上小寫的 `~/.config/lingocut`；若曾執行 `make dev`，還有 `~/Library/Application Support/lingocut Dev`。

## 開發

需要 Go 1.27.3 或更新版本。

```sh
make test   # go test for packages/core, packages/media and apps/kando
make dev    # run the app with live reload
make build  # build the app for this platform into apps/kando/build
```

`make dev` 會執行經 `tools/mygo-dev` 修補過的 MyGo `mygo dev`：在 macOS 上，重新載入時會保留正在執行的視窗，直到新的建置顯示出自己的視窗為止，而不是先把它關掉，所以儲存變更時看起來不會像應用程式結束了。如果 MyGo 升級後修補無法套用，它會發出警告並退回原本的 `go tool mygo dev`；此時請依照新的 `cmd/mygo/dev.go` 更新 `tools/mygo-dev/dev.go.patch`。背後的 `tools/mygo-dev/dev.sh` 是 POSIX shell 腳本。

螢幕截圖測試會從固定的示範資料庫把主要畫面算繪成 PNG 檔，並在空的模擬家目錄（home）中使用假的 `ffmpeg`、`yt-dlp` 和 `whisper-cli`，因此不會讀取你的設定、資料庫或工具：

```sh
KANDO_SCREENSHOTS=/tmp/kando-shots go test ./apps/kando/... -run TestScreenshots
```

設定 `KANDO_SCREENSHOTS_LANG`（`en`、`zh-Hans`、`zh-Hant`、`ja` 或 `ko`）會以對應的介面語言繪製，示範資料庫也會換成該語言讀者的內容；`-run TestScreenshotsReadme` 會繪製各語言 README 使用的四張圖，存放在 `docs/assets/<語言>/`。

存放庫結構：

- `apps/kando`：應用程式本身。每個畫面都有自己的目錄（`sidebar/`、`films/`、`film/`、`settings/`、`account/`、`cheatsheet/`）；`view.go` 把視窗與它們連接起來。`shelf/` 是工作記錄，`cloud/` 是 Google 登入與物件儲存上傳，`brand/` 是資料庫在 Finder 中的圖示。
- `packages/core`：與平台無關的工作、階段和連結解析。
- `packages/media`：媒體處理流程：下載、轉錄、翻譯、字幕匯出、封面和資料庫。
- `tools/mygo-dev`：`make dev` 背後使用的修補版 `mygo dev`。

[AGENTS.md](AGENTS.md) 說明介面如何拆分為元件，以及各區塊遵循的規則；修改介面之前請先閱讀。

## CI 與版本發布

[CI](https://github.com/yldm-tech/Kando/actions/workflows/ci.yml) 工作流程會建置並測試 Pull Request 和 `main`。[Release](https://github.com/yldm-tech/Kando/actions/workflows/release.yml) 工作流程會在 `main` 的 CI 通過後執行，也可以手動執行：它依照上一個標籤之後的 Conventional Commits 決定下一個版本號，以 `chore(release)` 提交到 `main` 並加上標籤，在不使用 Developer ID 或程式碼簽署憑證的情況下建置 macOS、Windows 和 Linux 的套件，再附上檢查碼發布 GitHub Release。

## 授權

[MIT](LICENSE)
