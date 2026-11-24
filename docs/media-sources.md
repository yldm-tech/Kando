# Kando 媒体来源设计（SMB、SFTP 与脚本来源）

> 本设计在 `docs/plugin-system.md` 的扩展点清单上加一个 E17「媒体来源」：让用户把 NAS、服务器或媒体服务器上的影片导入看懂，可以复制进资料库，也可以只建索引、直接从来源串流。owner 已定的事项列在 12.1。文中的行号在当前 main（`1182f64f`）上核对过，NSGo 的 API 以 `apps/kando/go.mod` 固定的 `v0.2.1-0.20261120114311-c94810a93e92` 的源码为准（下文写 NSGo 的路径都相对这个版本的模块根目录）。本文只是设计，不含实现代码。

## 1. 目标与非目标

### 目标

- 用户在看懂里添加「媒体来源」（一台 NAS 的 SMB 共享、一台服务器的 SFTP 目录，或者由插件提供的 WebDAV、Alist、Jellyfin、Emby），像浏览本机文件夹一样浏览它，选中影片导入。
- 每次导入由用户选两种方式之一：**复制进资料库**（和本机文件一样处理，离线可看，能同步到手机）或**只建索引**（不复制，从来源串流播放，识别时临时读取声音）。
- 内置的 SMB（SMB 2/3）和 SFTP 由看懂自己实现；基于 HTTP 的来源（WebDAV、Alist、Jellyfin、Emby）是官方脚本插件，第三方也能写新的来源。内置来源和插件来源经同一个注册表出现，界面一视同仁。
- 桌面和手机都支持。手机照旧从电脑拿影片，也能不经电脑直接连 NAS。
- 来源的密码、私钥、令牌只存在系统的安全存储里（`core.Secrets`），从不写进 settings.json，也从不交给插件的 JS。
- 遵守 `AGENTS.md` 的布局规则：媒体来源是一个新的块，纯逻辑放进非界面包，`package main` 里只放 `app` 方法，共享文件各加一行。

### 非目标

- 不把来源挂载成系统的网络盘，不依赖访达、资源管理器或 gvfs 的挂载。
- 不在来源上写任何东西：媒体来源只读。把影片「移到 NAS」仍是已有的「存储位置」（第 2 节），两者分开。
- 不做局域网发现（mDNS、NetBIOS 浏览）。NSGo 没有这类 API（第 13 节），第一版由用户填地址。
- 手机不运行插件代码（`docs/plugin-system.md` 第 1 节的非目标），所以手机直连只支持内置的 SMB 和 SFTP。
- 不支持 NFS、FTP、Kerberos 登录的 SMB 和 DLNA/UPnP。以后要加，按 E17 的同一个注册表加。

## 2. 现状：#302 已经有的东西

动手之前要知道，SMB、SFTP、WebDAV 的读写和回环串流在 `feat(storage): keep films on NAS shares and object stores, and play them from there (#302)` 里已经做了，服务的是另一个功能「存储位置」：把做好的影片的视频**移到** NAS，字幕留在资料库里，播放和手机同步从 NAS 读视频。本设计最大的工作量其实是把这些东西从「看懂自己挪过去的视频」推广到「用户原本就放在 NAS 上的视频」，而不是从零写协议。

| 已有的部分 | 位置 | 本设计怎么用 |
|---|---|---|
| `storage.Provider`：`Stat`、`List`、`Open(ctx, p, offset, length)`，路径都相对根目录，越界是 `ErrInvalidPath` | `apps/kando/storage/storage.go:16-33` | 内置来源的驱动直接包它，只用读的三个方法 |
| `storage.Remote` 和 `OpenRemote`：`KindSMB`、`KindWebDAV`、`KindSFTP` 三种连接 | `storage/remote.go:14-23`、`:33-46`、`:49-71` | 内置来源的连接配置就是它的 SMB 和 SFTP 两种 |
| SMB：`cloudsoda/go-smb2`，NTLM 登录，断线重连一次（`session[C]`） | `storage/smb.go:41-77`、`:79-102`，`storage/remote.go:101-176` | 照用 |
| SFTP：`pkg/sftp` 加 `x/crypto/ssh`，没有可信主机密钥就不连，`HostKeyError` 带服务器给的密钥和 SHA-256 指纹 | `storage/sftp.go:23-46`、`:154-216` | 照用，信任流程见 11.2 |
| `storage.Handler`：把 provider 的一个文件当成文件服务器来答，GET 和 HEAD、Range、If-Range、条件请求，内部用 `http.ServeContent` 加按需打开的 `rangeReader` | `storage/proxy.go:21-44`、`:47-99` | 串流服务器和手机同步都用它 |
| `storage.Proxy`：只听 `127.0.0.1:0`，每个文件一个随机 token，URL 是 `http://127.0.0.1:port/<token>/<文件名>` | `storage/proxy.go:101-158` | 第 7 节的回环串流服务器就是它的推广 |
| `storage.Pool`：按 ID 打开、复用、在设置变了时重开连接，`URL` 给播放器、`Handler` 给手机同步 | `storage/pool.go:53-148` | 来源照它的样子另有一个 `mediasource.Pool`（`mediasource/pool.go`），按来源 ID 持有驱动 |
| `storage.Get`：从 provider 复制到本机，先写隐藏文件、大小对上才改名 | `storage/transfer.go:53-54` | 复制进资料库用它 |
| 播放挪走的视频：`playerStored` 在后台 `pool.URL`，再把 URL 交给播放器 | `apps/kando/playback_stored.go:13-53` | 串流播放的写法照抄 |
| 手机同步挪走的视频：`phoneFilmOf` 给 `phonesync.Film.Remote` 一个 `pool.Handler` | `state_phonesync.go:418-426`，`phonesync/server.go:45-48`、`:626-642` | 串流的影片同样这样发给手机 |
| 设置 › 账号与上传 › 存储位置：添加、测试、首次连接 SFTP 时问是否信任（`account.AlertTrust`），密钥变了时警告（`AlertKeyChanged`） | `state_locations.go:19-55`，`account/location.go:26-38` | 来源的表单和信任对话框照它的样子做，第一期直接复用组件 |
| 测试：SFTP 有一个跑在回环上的假服务器（`fakeSSH`），SMB 只能接真服务器（`KANDO_TEST_SMB`，「there is no SMB server in Go to run here」），截图场景用 `locationsUI.open` 换掉真连接 | `storage/sftp_test.go:22`，`storage/smb_test.go:14-27`，`state_locations.go:32`，`screenshot_locations_test.go:17` | 第 19 节沿用 |

和 owner 的决定对照，有三处要说清楚：

1. **SMB 库**：owner 批准的是 `github.com/hirochachacha/go-smb2`，但仓库里已经用的是它的分支 `github.com/cloudsoda/go-smb2 v0.0.0-20261105190815-95be04a983b8`（`apps/kando/go.mod:12`）。上游最后一个版本是 `v1.1.0`（2022-03-12），分支在 2026-11-05 还有提交，两者都是 BSD-2-Clause。本设计建议**继续用已经在 go.mod 里的 cloudsoda 分支，不引入 hirochachacha**：同一个功能链接两份 SMB 实现没有意义，而且分支有上游没有的修复（例如 `Session.ListSharenames`，`cloudsoda/go-smb2@…/shares.go:104`，浏览一台 NAS 有哪些共享要用它）。是否同意见第 17 节第 1 条。`pkg/sftp` 已经是最新的 `v1.13.12`（2026-10-23），不用升级。
2. **凭据**：存储位置的密码、私钥和口令现在写在 settings.json 里（`state_locations.go:58`：「in settings.json, which saveSettings writes with mode 0600」）。媒体来源按 owner 的决定一开始就放进 `core.Secrets`，不沿用这个做法；存储位置要不要一起迁，作为单独的 PR 由 owner 决定（第 17 节第 2 条）。
3. **WebDAV**：`storage` 里已经有原生的 WebDAV provider（`storage/webdav.go`，`github.com/studio-b12/gowebdav v0.13.0`）。owner 定的是 WebDAV 作为官方脚本插件。本设计照这个决定写（WebDAV 是 P4 脚本来源 API 的参考插件，最适合用来验证 API），但要指出代价：手机不跑插件，所以手机直连 WebDAV 做不到，而原生实现几乎是现成的。owner 已定另加内置 WebDAV（第 17 节第 3 条）：P3 之后注册成内置种类，手机也能直连，插件仍做。

## 3. 扩展点 E17 媒体来源

照 `docs/plugin-system.md` 第 2 节的写法：

**E17 媒体来源（新，P0–P6，见第 20 节）**
- 接入位置：新的非界面包 `apps/kando/mediasource` 持有来源种类的注册表。内置的 SMB、SFTP 在 app 启动时注册；脚本插件在清单里声明 `contributes.sources`，载入后由插件块注册，停用、卸载时注销。界面（设置 › 媒体来源、浏览页、导入对话框）只读注册表，不分内置还是插件。
- 来源种类（`mediasource.Kind`）：`ID`（内置的是 `smb`、`sftp`；插件的是 `plugin:<插件 id>:<来源 id>`）、`Title`（多语言）、`Icon`、`Fields`（连接表单的字段，见 3.2）、`Phone bool`（能不能在手机上直连，只有内置的是 true）、`Origin`（内置或哪个插件，详情页和来源列表上写出来）。
- 驱动（`mediasource.Driver`）是只读的窄接口，所有种类都实现它：

  ```go
  // Driver reads one media source. Paths are slash-separated and relative to the source's root, "" being the root.
  type Driver interface {
  	List(ctx context.Context, dir string) ([]Entry, error)
  	Stat(ctx context.Context, p string) (Entry, error)
  	// Open reads p from offset, length bytes or to its end with -1, as storage.Provider.Open does.
  	Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error)
  	Close() error
  }

  // Entry is a file or folder of a source. Version is what tells a changed file from the one imported: an ETag, or size and time.
  type Entry struct {
  	Name     string
  	Size     int64
  	Modified time.Time
  	Dir      bool
  	Version  string
  }
  ```

  内置驱动把 `storage.Provider` 包一层（写、删、移动不暴露）；插件驱动的 `List`、`Stat` 调插件的导出函数，`Open` 由宿主按插件给的 HTTP 地址带 Range 去取（11.3），视频字节不经过 JS。
- 驱动的语义和 `storage.Provider` 一致（`ErrNotExist`、`ErrInvalidPath`），所以 `storage.Handler`、`storage.Get` 和 `rangeReader` 只需把参数类型从 `Provider` 放宽成只含 `Stat` 和 `Open` 的接口（`storage.Reader`），就能同时服务存储位置和媒体来源。这是 P0 的一步重构，行为不变。
- 不改：`storage` 的写路径、存储位置的设置和 library.json 的 `stored`；`media.Pipeline` 接口；`a.pipeline.(media.Tools)` 断言（非测试文件约 73 处，见 `docs/plugin-system.md` 第 1 节）。

### 3.1 来源模型

一个来源（`mediasource.Source`）是用户添加的一条连接：

- `ID`：随机的 16 位十六进制，创建时生成，之后不变。影片按它认来源（第 6 节）。
- `Kind`：注册表里的种类 ID。
- `Name`：用户起的名字（「客厅 NAS」），默认取主机名。
- `Config`：不是秘密的字段，按种类的 `Fields`：SMB 是 `host`、`port`（默认 445）、`share`、`root`、`user`、`domain`，以及第 17 节第 4 条的「不支持签名」开关 `noSigning`；SFTP 是 `host`、`port`（默认 22）、`root`、`user`、`hostKey`（信任过的主机密钥，authorized_keys 的一行，见 12.2）；插件来源是插件声明的字段加上 `endpoint`（用户填的服务器地址，见 11.3）。
- `Secrets`：秘密字段的名字（SMB 的 `password`；SFTP 的 `password`、`privateKey`、`passphrase`；插件声明为 `secret` 的字段），值不在这里。

存放：

- settings.json 里只存 `savedMediaSources{MediaSources []savedMediaSource}`（新的设置部分，键 `mediaSources`，带自己的 `clean()`，按 `settings_file_test.go` 的规则不能和别人重名、不能自定义 Marshal）。每项是 `{id, kind, name, config}`，`config` 是字符串 map，`clean` 丢掉种类不认识的字段。插件来源的插件暂时缺席时（停用、卸载、盘没挂上）保留这一项，和 `savedLLM.clean` 保留 `pack.` 前缀服务的道理一样（`docs/plugin-system.md` E4），否则已导入影片的来源会丢。
- 秘密存在 `core.Secrets` 的分区 `mediasource`（`cx.ProvideSecrets("mediasource")`，NSGo 把服务名写成 `<Service>#mediasource`，`core/secrets.go:56-63`、`:74`），账号是 `<来源 ID>.<字段名>`。插件来源也存在这个宿主分区里，不在插件的 `plugin.<id>.<signer>` 分区，插件的 JS 读不到（`docs/plugin-system.md` 6.3）。插件来源的秘密另外绑定 `endpoint` 的 origin：存的时候一起存 `<来源 ID>.origin`，用的时候 origin 不一致就当作没存，用户改了地址就要重新输入密码，免得一个插件被骗着把旧密码发到新地址。
- `core.Secrets` 在 macOS、iOS 是钥匙串，Windows 是凭据管理器，Linux 是 Secret Service，Android 是 AndroidKeyStore 加密的文件（`internal/platform/apple/secrets.go:18-20`、`windows/secrets.go:14`、`linux/secrets.go:18`、`android/secrets.go:12`）。Linux 上没有 Secret Service 时 NSGo 返回 `core.ErrSecretsUnavailable`，并且「NSGo never keeps a secret in plain storage instead」（`core/secrets.go:12-13`）。看懂同样不退回明文：这时需要密码或私钥的来源不能添加（实际只剩 SMB 的 guest 连接），表单上写明原因。
- 日志和反馈 issue：来源的秘密加入 `feedbackSecrets`（`state_feedback.go:310-317`）要遮掉的列表；来源的主机名、共享名和路径算路径信息，按 `feedbackScrubber` 换成 `<路径>`。

### 3.2 种类声明的字段

`Fields` 是声明式的表单，宿主统一画（`settings/form` 的组件），插件不画自己的连接界面：

- `type`：`text`、`number`、`secret`、`bool`、`url`（只用于 `endpoint`）、`hostKey`（只有内置种类有，最多一个：SFTP 信任的服务器密钥，表单上显示指纹，只经首次信任和「替换密钥」的对话框改，不能手填）。`text` 和 `secret` 可以标 `multiline`（私钥的 PEM 文本），多行显示所填内容，不遮挡。
- `key` 匹配 `^[a-z][A-Za-z0-9]{0,31}$`；最多 12 个字段，`secret` 最多 3 个。
- 每个 `secret` 字段声明它怎么用（`auth`），宿主据此注入，插件拿不到值：`{"basic": ["user", "password"]}`（HTTP Basic，用户名取另一个字段）、`{"header": "X-Emby-Token", "format": "{secret}"}`，或 `{"login": true}`（见 11.3：只在插件调登录接口时由宿主填进请求体）。

## 4. 浏览

### 4.1 桌面

- 侧栏「资料库」一组下面加一行「媒体来源」（只在有来源时显示，没有时设置里有入口），点开是浏览页 `apps/kando/sourcebrowser/`（新的界面目录）。页面左边是来源列表（名字、种类图标、内置或「插件 · <插件名>」），右边是当前目录：面包屑、文件夹在前、按名字排序，只列视频、声音文件（`media` 认得的扩展名）和文件夹，旁边同名的字幕文件（`.srt`、`.ass`、`.vtt`）不单独列，而是在视频那一行标一个「有字幕」的徽标，导入时一起带上（复制方式复制它，索引方式读它）。
- SMB 来源没有填共享名时，第一层列出这台机器的共享（`Session.ListSharenames`），用户点进一个共享再往下走；选好后可以把它存成来源的 `share`。
- 每一行：名字、大小、修改时间；已经导入过的（按第 6 节的身份）标「已在资料库」，点它打开影片。多选后底部出现「导入 N 个」，打开导入对话框（`ui.Dialog`，经 `overlays()` 的 `mediaSourceOverlay`）：两个单选项「复制到资料库」「只建索引，从来源播放」，下面一行写清楚差别（「复制后离线也能看，占用这台电脑 X GB」「不占空间，播放和识别时需要连得上 <来源名>」），再加一个「记住我的选择」。确认后调 `Enqueue`（复制方式）或 `mediaSourceLink`（索引方式，见 5.2）。
- 也可以把整个文件夹导入：文件夹那一行的右键菜单「导入这个文件夹」，递归最多 3 层、最多 500 个文件，超过时说明并只导入前 500 个。
- 列目录在后台 goroutine 里做，经 `a.update` 回到引擎；构建界面的函数里不连接、不读盘（`AGENTS.md`）。正在列时显示进度，连接失败时页面上写 `storage.Diagnose` 给出的原因（`storage/diagnose.go:59`，密码错、主机不通、证书或主机密钥不对），带「重试」和「编辑来源」。
- 设置里新增一节「媒体来源」（`settings/sectionid` 加 `MediaSources = "sources"`，`settings/mediasources.go`；Go 名字不叫 `Sources`，因为 `settings.Sources`、`SourcesApp`、`Window.Sources` 和 `settings/sources.go` 已经是翻译页的「翻译来源」），列出来源，「添加来源…」先选种类（注册表里的全部种类，插件来源排在内置之后，写「插件 · <插件名>」），再填 3.2 的表单，「测试」连接一次并列根目录。表单、测试结果和 SFTP 的信任对话框照搬存储位置的 `account.Draft`、`locationTest`、`AlertTrust`、`AlertKeyChanged`（`state_locations.go:19-55`），不另写一套。
- 存储位置和媒体来源常常是同一台 NAS。添加来源时，如果已有存储位置，表单上给一个「从存储位置填入」的下拉框，只复制主机、端口、共享、用户这些非秘密字段，密码仍要用户输入（因为存储位置的密码在 settings.json 里，不能挪进来源再说它从没进过 settings.json）。

### 4.2 手机

- 「资料库」页面加一个「NAS」分组（只在手机自己添加了来源时出现），每个来源一行；点进去是和桌面同样的目录列表（`apps/mobile/sources/`，新的界面目录）。手机屏幕窄，不并排，来源列表和目录是两层页面。
- 行的操作：点视频直接播放（串流，第 9 节）；左滑或长按出现「下载到手机」「从手机移除下载」。已经下载的标一个对勾，和电脑同步来的影片一样显示下载进度。
- 添加来源在手机的设置里：「NAS 与服务器」一节，只有 SMB 和 SFTP 两种。iOS 上第一次连局域网时系统会问本地网络权限，`apps/mobile/nsgo.yaml` 已经有 `NSLocalNetworkUsageDescription`（手机同步要用），但文案只说了连电脑，要改成也包括 NAS（五份 catalog 的 InfoPlist.strings 一起改）。

## 5. 两种导入方式

### 5.1 复制进资料库

- 导入时为每个文件调 `Enqueue(ref)`，`ref` 是第 6 节的来源地址 `kando-source://<来源 ID>/<路径>`。之后的一切和本机文件一样：去重（`queueSourceKey`）、排队、识别、翻译、离线、同步到手机。
- 下载这一步：`packages/core` 的 `ParseLink` 认出 `kando-source://`，平台是新加的 `core.PlatformSource`（`packages/core/link.go:18-20` 旁边）。`media.Tools.Download`（`packages/media/tools.go:313`）对这个平台不走 yt-dlp，而是调 `Tools` 新增的可选字段 `Sources media.SourceReader`（和 E14 的 `Engine` 一样是可选字段，不改任何 `media.Tools` 字面量和断言）：

  ```go
  // SourceReader is how a job reaches a video on a media source (kando-source:// links): Copy copies it into dir with progress and returns the file; URL is where ffmpeg and the player read it now, the loopback stream server's address.
  type SourceReader interface {
  	Copy(ctx context.Context, source, dir string) (string, error)
  	URL(ctx context.Context, source string) (string, error)
  }
  ```

  P1 已经落地的 `media.SourceReader`（`packages/media/source.go`）只有 `Copy`，`URL` 在 P2 做索引方式时再加。`Copy` 的实现是非界面包里的 `mediasource.Copier`（`mediasource/copy.go`），`package main` 只在 `state_mediasource_browse.go` 的 `mediaSourceCopier` 把 Pool、离线开关和 `Local` 交给它，`queuePipeline` 赋给 `Tools.Sources`：按来源 ID 从 `mediasource.Pool` 取驱动，用 `storage.Get` 复制到资料库里的 `<资料库>/<来源名>/<来源里的路径>`（和 6.3 的占位路径同一个 `mediasource.Placeholder`，重名时加序号），复制的进度经 `media.ReportProgress` 报给 `media.Trace`，所以卡片上显示「下载中 42%」和速度，和下载网上的视频一样。同名的字幕（`mediasource.SubtitlesOf`）按复制后的视频改名一起复制，交给已有的「本机视频旁边有字幕」那条路径处理。离线模式下互联网上的来源以 `media.ErrOffline` 失败（12.4）。
- 复制完的文件就是资料库里的普通视频，`core.Job.Source` 仍是来源地址，用来去重和在卡片上写「来自 <来源名>」。之后来源删掉、连不上都不影响这部影片。
- 离线模式：`Tools.Download` 在离线时对非本机来源返回 `ErrOffline`（`tools.go:320-322`），`privacyRefusesLink` 也拒绝非本机链接（`state_privacy.go:185-190`）。媒体来源按 12.4 的规则：主机是局域网地址的来源在离线模式下照常可用，所以这两处要认出 `PlatformSource` 并问来源的地址，而不是一律拒绝。

### 5.2 只建索引（串流）

- 导入时不复制，`mediaSourceLink(refs...)` 为每个文件在资料库里建一条记录（第 6 节），立刻出现在资料库里，海报和时长在后台用 ffmpeg 从串流地址读（第 8 节）。
- 然后照常排一个任务做字幕：`core.Job` 加一个 `Linked bool`（library.json 的 `linked`，旧版读到会忽略），`VideoPath` 是第 6 节的占位路径。`media.Make` 对 `Linked` 的任务不下载，`hearFrom` 取 `Tools.Sources.URL(source)`，其余步骤不变。
- 用户以后想离线看，影片的 ⋯ 菜单里有「复制到资料库」：把视频复制到占位路径上，删掉 `linked` 记录，它就成了普通影片；反过来普通影片不提供「只留索引」（那是存储位置「移到 NAS」做的事，而且会删掉本机文件，两件事不混）。

## 6. 远程影片的身份

### 6.1 来源地址

`kando-source://<来源 ID>/<路径>`，路径按段 `url.PathEscape`，相对来源的根目录。它只在看懂内部用，不是可以打开的网址；`ParseLink` 给它 `Platform: PlatformSource`、`Kind: LinkVideo`、`ID: <来源 ID>/<路径>`。来源 ID 是随机的，所以来源改名、换 IP 都不影响影片；地址里没有主机名和用户名，存进 library.json、发给手机都不泄露 NAS 的地址。

### 6.2 library.json

复制进来的影片不需要新字段（`Source` 就是来源地址）。索引影片在 `shelf.Library` 加一个和 `Stored` 并列的 `Linked map[string]Linked`（`shelf/library.go:44-45` 旁边），键是占位路径，值是：

- `Source`：来源 ID；`Path`：来源里的路径；
- `Size`、`Modified`、`Version`：导入时 `Stat` 到的，用来发现文件变了（6.4）；
- `Duration`：时长，文件不在本机，`storedLengths` 那样从这里恢复（`state_stored.go:137-142`）；
- `Added`：导入的时间。

为什么不直接复用 `Stored`：`Stored` 的意思是「看懂自己挪过去的副本」，删除影片时会连同 NAS 上的副本一起删（`films/remove.go:17`、`:38`），还有「移回资料库」；索引影片的文件是用户自己的，看懂永远不写、不删、不移动它。两种记录分开，删除、移动的代码不可能误伤用户的文件。读写代码可以共用（`storedOf`、`playerStored`、`phoneFilmOf` 的串流分支都改成先问 `Stored` 再问 `Linked`），这是 8.4 那种接点上的最小改动。

旧版看懂不认 `linked`，下次保存时会丢掉它（`shelf/library.go:326` 的注释说 `stored` 也是这样），这些影片在旧版里就只剩字幕。这和 `stored` 的兼容性一样，写进发版说明。

### 6.3 占位路径

字幕、转写稿、更正、术语这些伴随文件（`media.Companions`）都按视频路径命名、放在视频旁边。索引影片的视频不在本机，所以给它一个资料库里的占位路径：`<资料库>/<来源名>/<来源里的路径>`（来源名和路径里不能做文件名的字符替换掉，重名时加序号），只建目录、不建视频文件。资料库刷新时，`films.MovedVideos`（`films/model.go:109`）照它列挂走的视频那样也列出 `Linked` 的占位路径，所以影片一直在资料库里。占位路径之后不跟着来源改名而改，记录里存的是来源 ID。

### 6.4 来源离线、删掉、文件变了

- **连不上**（NAS 关机、不在家里的网络）：资料库照常列出影片，字幕、学习、搜索、导出都照常（它们读的是资料库里的伴随文件）。播放时 `playerStored` 那样在后台打开，失败就在播放页显示「连不上 <来源名>」和「重试」，只显示字幕，和视频丢了时一样（`playback_stored.go:40-49` 的 `playErrStoredUnreachable`）。海报和列表行上的标记照 `StoredMark` 的做法只看设置和上一次的结果，绑定里不连网。
- **来源从设置里删掉**：已导入的影片保留，标记写「来源已删除」；照 `locationRelink` 的做法（`state_locations.go:19`），删掉的来源只保留非秘密的配置（`removedMediaSources`），以后用同样的种类和地址加回来时拿回原来的 ID，这些影片又能播。删除来源的确认框写明「有 N 部影片只建了索引，删除后它们只剩字幕」。
- **文件变了**：每次打开播放、做字幕之前 `Stat` 一次，大小或 `Version` 和记录不同时，播放照常，但播放页顶上提示「<来源名> 上的这个文件变了，字幕可能对不上」，给「重新做字幕」和「知道了」（后者把记录更新成新的版本）。文件不在了（被移动或删掉）时显示「在 <来源名> 上找不到这个文件」，给「在来源里找」：打开浏览页定位到原来的目录，用户选一个文件后只改记录的 `Path`。

## 7. 串流播放

### 7.1 `core.Video` 能接受什么

固定版本的 `core.Video.Source` 是「a URL, a file path or an "asset:" path」（`core/video.go:13-15`），传到平台层的 `driver.VideoConfig` 只有 `Source string` 和播放开关（`internal/driver/mount.go:1448-1461`），**没有请求头、cookie，也没有让 app 提供数据的 reader 或 resource loader**。各平台：macOS、iOS 是 `AVURLAsset URLAssetWithURL:options:`（`internal/platform/apple/video.go:225-231`、`:255`）；Windows 是 `IMFMediaEngine::SetSource`（`windows/video.go:233-235`）；Linux 是 `GtkMediaFile`（`linux/video.go:97-106`、`:125`）；Android 是 `MediaPlayer.setDataSource(context, Uri)`，没用带 headers 的重载（`internal/platform/android/java/dev/oneo/nsgo/VideoNode.java:219-242`）。

所以 SMB、SFTP 这类不是 HTTP 的来源，以及要带认证头的 HTTP 来源，都必须经一个 app 自己的本机 HTTP 服务器交给播放器。这正是 #302 的 `storage.Proxy` 已经在做的事。带请求头的需求上游已有 [nsgo-dev/nsgo#681](https://github.com/nsgo-dev/nsgo/issues/681)（「core.Video: request headers and a separate audio stream」，还开着），但即使它做了，SMB 和 SFTP 也仍要回环服务器，所以本设计不依赖它（第 13 节）。

### 7.2 回环串流服务器

把 `storage.Proxy` 推广成服务任何 `storage.Reader`（3 节的 P0 重构），来源和存储位置共用一个实例（`storage.Pool` 持有）。规则：

- 只听 `127.0.0.1` 的随机端口（`net.Listen("tcp", "127.0.0.1:0")`，`storage/proxy.go:118-127`），不听 `0.0.0.0`、不听 IPv6 的 `::`。
- 每个文件一个随机 token（`crypto/rand` 的 `rand.Text()`，26 个 base32 字符，约 130 位），在 URL 路径的第一段；不认识的 token 一律 404，不区分「没有」和「不许」。同用户的其他程序只能读到它被给过 URL 的文件，读不到来源里别的文件，也列不了目录（服务器根本没有列目录的路由）。
- 新增两条加固，P0 一起做（现有的 `Proxy` 没有）：
  - 检查 `Host` 头必须是 `127.0.0.1:<端口>`，否则 421。这防的是 DNS rebinding：浏览器里的网页把自己的域名解析到 127.0.0.1 后，即使猜不到 token，也不让它和服务器说话。
  - token 有生命周期：播放器换影片、关闭播放页时，`Pool` 撤销这部影片的 token；任务用的 token 在任务结束时撤销。现在的 `Proxy.URL` 对同一个文件永远给同一个 URL（`proxy.go:130-146`），直到退出。撤销后旧 URL 立刻 404。
- 只答 GET、HEAD，支持 Range、If-Range、条件请求（`storage.Handler`），不加任何 CORS 头，所以网页里的脚本即使拿到 URL 也读不到响应。
- 响应头：`Content-Type` 按扩展名（`psync.MediaType`），`Cache-Control: no-store`，不带 `Content-Disposition`。
- 来源连不上时，打开阶段在 `PoolOpenTimeout`（30 s，`pool.go:50`）内失败，回 502，播放器报错，播放页显示 6.4 的「连不上」。播放中断线时 `session[C].do` 重连一次（`storage/remote.go:147-162`），仍失败就让这次读失败，播放器自己会重试 Range 请求。

### 7.3 播放的接法

`playerStored`（`playback_stored.go:13-53`）的写法原样推广：`startPlayback` 先问 `a.storedOf(path)`，再问 `a.linkedOf(path)`，有就在后台 `pool.URL` 拿到回环地址，经 `a.update` 写进 `p.source`，`core.Video` 的 `SourceFunc` 读到新值就开始播放。不在构建界面时连网（`AGENTS.md`）。

## 8. 流水线

- **复制方式**：下载步骤换成 `Tools.Sources.Copy`（5.1），之后和本机文件完全一样。
- **索引方式**：`media.Make` 对 `job.Linked` 的任务走 `local` 那一支（`packages/media/make.go:158-176`），但不调 `Download`，`hearFrom` 是 `Tools.Sources.URL(job.Source)` 给的回环地址。这是 `packages/media` 要做的唯一改动：`SourceReader` 接口、`Tools.Sources` 字段、`Download` 认 `PlatformSource`、`Make` 认 `Linked`，不改 `Pipeline` 接口和 `Run` 的语义。
- **ffmpeg 从哪里读**：从回环串流服务器的 `http://127.0.0.1:<端口>/<token>/<文件名>`。`Tools.Hear` 本来就是一段一段地 `ffmpeg -ss <起点> -i <输入> -t <长度> -vn -ac 1 -ar 16000 … audio.wav`，把这一段的声音解到临时目录、交给 whisper、用完删掉（`packages/media/hear.go:22-44`），输入换成 URL 后，ffmpeg 的 http 协议用 Range 跳到起点，所以「识别时临时读取声音」不需要先把整个视频拉下来，临时文件只有当前这一段的 wav。时长（`Tools.Length`，`make.go:23-28`）、海报（`poster.go:76`、`:151`）、内嵌章节（`embedded_chapters.go:23`）和内嵌字幕也都是 `ffmpeg -i <路径>`，同样直接换成 URL。
- **网络开销要说清楚**：视频和声音在容器里是交错存放的，ffmpeg 抽声音时要读过整个文件，所以给一部索引影片做一次字幕，大约从 NAS 读一遍整个文件（`Make` 一边识别一边让用户看，#306 的边播边做也照常）。翻译、校对、术语都不读视频，不产生流量。这写在导入对话框的说明里。
- **ffmpeg 的 http 支持**：看懂用的 ffmpeg 可能是它自己下载的一份，也可能是系统的（`state_tools.go:15`），常见的构建都带 http 协议，但不保证。P2 加一条检查：`ffmpeg -protocols` 里没有 `http` 时，索引方式的做字幕在 `Check` 阶段失败，卡片说「这个 ffmpeg 不能读网络地址」并给「改用看懂自带的」（`toolsUseOwn`，`state_tools.go:210`）。播放不受影响（播放器自己读 URL）。
- **重新处理**：存储位置挪走的视频现在拒绝重新处理（`storedNeedsFile`，`queue.go:479`、`:507`）；索引影片不拒绝，重新处理照样从回环地址读。
- **导出带声音的句子**：`saved.SoundFor` 用 ffmpeg 切（`docs/plugin-system.md` 5.3），输入换成回环地址；来源连不上时和缺 ffmpeg 一样在导完的说明里提醒哪些句子没有声音。
- **离线模式**：索引影片按 12.4 的规则判断来源能不能用，不能用时任务在 `Check` 阶段以 `media.ErrOffline` 失败，卡片指向隐私设置。

## 9. 手机同步（电脑上的索引影片）

- 电脑把索引影片当成存储位置的影片发给手机：`phoneFilmOf`（`state_phonesync.go:418-426`）在 `Stored` 之外再查 `Linked`，来源还在设置里时给 `phonesync.Film.Remote` 一个 `pool.Handler`，`RemoteSize`、`RemoteAt` 用记录里的大小和导入时间；来源已删除时按只有字幕的影片发（`SubtitlesOnly = true`）。手机端不用改：它照旧经 `/v1/films/{id}/video` 按 Range 下载（`phonesync/server.go:626-642`，`apps/mobile/download/download.go:123-191`），电脑在中间从 NAS 读、经 TLS 发给手机。
- 手机拿到的是什么：字幕、海报、逐词标注照常；视频由电脑转发，所以**电脑和 NAS 都要在线**，电脑连不上来源时手机收到 502，下载页显示「电脑连不上这部影片的来源」。`playable`（`server.go:757-793`）对 `Remote` 影片用记录的大小和时间，不 `Stat`，所以列影片时电脑不会因为 NAS 关机而卡住。
- **外出时的存储中转**：手机离开家里的网络时经 R2 中转拿影片（`packages/sync/relay.go`、`relay_films.go`），但电脑上传到桶里的只有本机磁盘上的视频（`phonesync/films.go:528-531` 的 `fileOf` 对不在磁盘上的视频返回 false），挪走的视频和索引影片都不上传。这是 #302 就有的缺口，本设计不改它，只在手机上把这类影片标成「只在家里的网络可下载」。要不要让电脑从 NAS 读出来再上传，见第 17 节第 6 条。
- **协议**：不需要新的调用。`psync.Film`（`packages/sync/protocol.go:92-117`）的 `Source` 是侧栏分类（youtube、bilibili、web、local），索引影片和复制进来的影片都发 `local`，旧手机不会看到不认识的分类。另加一个可选字段 `SourceKey`（`omitzero`，见 10.5），手机直连时用它认出 NAS 上的同一个文件；照 `TestWordSourceIsAnAddition` 的写法加 `TestSourceKeyIsAnAddition`，不升协议版本（`packages/sync/v2.go:10` 的规则）。

## 10. 手机直连

### 10.1 共享代码

手机要说 SMB 和 SFTP，就要有和电脑同一份协议代码。现在它在 `apps/kando/storage` 里，手机不导入 `apps/kando` 的包（`apps/mobile/go.mod` 只 require `packages/core`、`study`、`sync`）。所以 P5 先把 `storage` 里和协议有关的部分（`Provider`、`Remote`、`OpenRemote`、smb、sftp、`Handler`、`rangeReader`、`Proxy`、`Diagnose`）挪进新的 `packages/remotefs`，`apps/kando/storage` 只留对象存储、本机文件夹、`Pool`、`Put`/`Get`，从 `remotefs` 导入其余的。`proxy.go` 对 `apps/kando/cloud` 的唯一依赖是 `cloud.ContentType`，它本来就是 `psync.MediaType`（`apps/kando/cloud/store.go:86`），改成直接用后者。挪的时候只导出别的包用到的标识符，测试跟着走（`AGENTS.md` 最后一条）。`packages/remotefs` 的 `go.mod` 引入 `cloudsoda/go-smb2`、`pkg/sftp`、`x/crypto`，`apps/mobile/go.mod` 随之多出这几个依赖（第 16 节）。

### 10.2 存放

- 来源配置：手机的 store 加 `sources.json`（`apps/mobile/store` 的数据目录下，和 `prefs.json`、`links.json` 并列，`store/store.go:78-87`），每项 `{id, kind, name, config}`，和电脑同形。
- 秘密：`cx.Secrets()`，账号 `source.<来源 ID>.<字段>`，和配对令牌一样用 `core.SecretAfterFirstUnlockThisDeviceOnly`（`apps/mobile/session/session.go:205-207`），不进 iCloud 钥匙串、不随备份走。
- 下载下来的影片：`films/source-<来源 ID>/<影片 ID>/`，和电脑来的 `films/<电脑>/<影片>/` 同一种布局（`film.json`、`video.part` 写完改名成 `video<扩展名>`），影片 ID 是来源路径的 SHA-256 前 16 位，所以同一个文件不会下载两份。`store.Film` 的 `Desktop` 为空，新加 `Source`（来源 ID）和 `Path`。

### 10.3 播放和离线缓存

- 串流：手机上也起一个回环串流服务器，`core.Video.Source` 是 `http://127.0.0.1:<端口>/<token>/<文件名>`。手机上已经有一个：`apps/mobile/loopback`，边播边生成用它把系统播放器的请求转给电脑（`sync.Client.ServeVideo`）。P5 让它也服务 `remotefs` 读出来的字节，不另起一个。
  - iOS：AVFoundation 读 `http://127.0.0.1` 受 App Transport Security 约束；`apps/mobile/nsgo.yaml` 的 `apple.info` 已经写了 `NSAppTransportSecurity: {NSAllowsLocalNetworking: true}`。够不够用（还是要 `NSAllowsArbitraryLoadsForMedia`），要在真机上确认。
  - Android：targetSdk 35 默认拒绝明文 HTTP。NSGo 已经补上只对回环放开的选项（nsgo#1061，nsgo PR #1153 的 `android.loopback_cleartext`），`apps/mobile/nsgo.yaml` 已经打开，所以 Android 上也能串流。
- 下载到手机：`apps/mobile/download` 的 `Source` 接口（`download/download.go:20-24`）现在由 `*sync.Client` 和 `sync.RelayFilms` 实现，再加一个来源的实现，`video` 的续传（`.part` 的大小作为起点、`If-Range` 带 `Version`，`:123-191`）照用，对来源来说就是 `Driver.Open(path, offset, -1)`。
- 字幕：手机不识别语音，直连 NAS 的影片本身没有字幕。两种办法给它字幕，都在 P5：
  - NAS 上视频旁边有字幕文件时，下载或串流时一起读。
  - 电脑做过同一个文件的字幕时，从电脑拿：手机按 `SourceKey` 匹配（10.5），匹配上就像电脑来的影片一样经局域网或存储中转取字幕和标注，播放时视频从 NAS 来、字幕从电脑来。
- 让电脑给 NAS 上的文件做字幕：手机的 ⋯ 菜单「在电脑上做字幕」，经 v2 已有的 `jobs.create`（`psync.OpJobCreate`，`packages/sync/v2.go:53-54`）请电脑做，`JobCreate` 加一个可选的 `Source`（种类、主机、端口、共享、路径、大小，不含用户名和密码），电脑按地址找自己的同一个来源（没有就回 `not_found`，手机提示先在电脑上添加这个 NAS）。这一步依赖电脑认得手机给的地址，放在 P5 的最后一个 PR，可以不做（第 17 节第 7 条）。

### 10.4 手机上不支持的

- 插件来源：手机不跑插件代码，所以 WebDAV、Alist、Jellyfin、Emby 在手机上只能经电脑（第 9 节），手机的「添加来源」里没有它们。
- 局域网发现：同第 1 节。

### 10.5 `SourceKey`

一个文件在 NAS 上的身份不能用来源 ID（电脑和手机各自添加来源，ID 不同），而要用地址：`SourceKey = base64url(SHA-256(种类 ‖ 小写主机 ‖ 端口 ‖ 共享 ‖ 路径 ‖ 大小))` 的前 22 个字符。用哈希而不是明文，`psync.Film` 里就不出现 NAS 的主机名和路径。主机名不同（`nas.local` 和 `192.168.1.10`）会认不出来，这是有意的保守：认错一部影片比认不出更糟。

## 11. 插件的脚本来源 API

### 11.1 清单

脚本插件的 `contributes` 加 `sources`（`plugins.Contributes`，`apps/kando/plugins/script.go:37-45`），最多 4 项：

```json
"capabilities": {"host": ["kando.source.fetch"]},
"contributes": {
  "sources": [{
    "id": "webdav",
    "title": {"en": "WebDAV", "zh-Hans": "WebDAV"},
    "icon": "server",
    "fields": [
      {"key": "endpoint", "type": "url", "title": {"en": "Server address", "zh-Hans": "服务器地址"}},
      {"key": "user", "type": "text", "title": {"en": "User", "zh-Hans": "用户名"}},
      {"key": "password", "type": "secret", "auth": {"basic": ["user", "password"]}, "title": {"en": "Password", "zh-Hans": "密码"}}
    ]
  }]
}
```

- `id`、`title` 规则同其他贡献块；`icon` 只能是新的 `plugins.SourceIcons`（`server`、`hard-drive`、`cloud`、`film` 这几个 Lucide 名）之一，和 `plugins.PanelIcons`（`plugins/contrib.go:75`）一样由看懂自己画，不收插件的图片。
- `fields` 按 3.2，必须有且只有一个 `type: "url"` 的 `endpoint`。
- 声明了 `sources` 的插件**必须申请 `kando.source.fetch`，并且不能申请任何别的网络出口**：`capabilities.network` 必须为空，不能有 `credentials`、`kando.net.credentialFetch`、`kando.llm.chat`、`kando.ui.openURL`，也不能申请读字幕的 `kando.lines.seen`，`plugins.Validate` 拒收（新的哨兵错误 `ErrSourceNetwork`）。理由见 11.4：这样来源插件读到的文件列表、登录得到的会话令牌只能回到用户填的那台服务器和看懂，发不到别处。
- 和其他贡献块一样受签名清单或 `kandoManifest()` 的比对保护（`docs/plugin-system.md` 4.3），详情页「添加的功能」列出「媒体来源：<标题>」。

### 11.2 导出函数

宿主调用，插件没在运行时不调（同 `docs/plugin-system.md` 5.3 的贡献点函数）：

- `list(source, dir)`：回 `[{name, dir, size, modified, version}]`，最多 5000 项，`name` 不能含 `/`、`\`、NUL，不能是 `.`、`..`；`modified` 是 RFC 3339。
- `stat(source, path)`：回一项或 `null`（不存在）。
- `open(source, path)`：回 `{path, query, header}`，`path` 和 `query` 相对来源的 `endpoint`；或 `{url, header}`，`url` 必须和 `endpoint` 同 origin。另可回 `ttl`（秒，默认 600，最多 86400）：宿主在这段时间内复用这个结果，过期、或请求回 401、403 时再调一次 `open`。
- `source` 是 `{id, config}`：`id` 是按插件派生的不透明 ID（和 `docs/plugin-system.md` 5.3 的影片 ID 一样用 `HMAC(pluginSecret, 来源 ID)`），`config` 是非秘密字段，**没有** `secret` 字段的值。
- 每次调用等 15 s，超时按 `timeout` 失败；回答形状不对算 `bad-reply`。失败带码：`auth`（认证失败，宿主提示重新输入密码）、`not-found`、`unavailable`，其余同 E14 的失败码。

### 11.3 宿主 handler `kando.source.fetch`

插件列目录、查文件、登录都只能经它：

- 参数 `{source, method, path, query, header, body, login}`。`method` 只能是 GET、HEAD、POST、PROPFIND、REPORT；`path` 相对 `endpoint` 的路径，拼完后必须仍在 `endpoint` 的路径前缀下（按段比较，`..` 拒收）；`query` 是字符串 map；`header` 最多 16 个，不能是 Host、Cookie、Content-Length、Range 这类由请求本身或看懂决定的头；Authorization 可以（插件用它放登录得到的会话令牌），来源声明了 `basic` 或 `header` 注入时被宿主注入的值覆盖；`body` 最多 1 MiB。
- 宿主把 `secret` 字段按声明注入：`basic` 写 `Authorization: Basic …`，`header` 写指定的头（覆盖插件给的同名头）；`login: true` 时，请求体里的 `{{secret:<字段名>}}` 换成那个秘密（只用于登录接口，例如 Alist 的 `POST /api/auth/login`、Jellyfin 的 `POST /Users/AuthenticateByName`），换完的请求体不回给插件。
- 回 `{status, header: {"Content-Type", "ETag", "Last-Modified", "Content-Length"}, body}`，body 最多 4 MiB（和 `credentialFetch` 的 `pluginsFetchReplyMax` 一样，`state_plugins_calls.go:37`、`:260`）。不跟随重定向，`Set-Cookie` 丢掉，和 `credentialFetch` 相同（`state_plugins_calls.go:294-303`）。
- `endpoint` 的 scheme：https 一律可以；http 只在主机是回环、私有、链路本地地址或 `.local`、`.lan` 名字时可以（局域网里的 NAS 多半只有 http），这时表单在地址下面写「这个地址不加密，密码会以明文在局域网上传送」。
- 这是看懂宿主替插件发请求，不走 tsx 自己的 `fetch` 和签名的 `network` 规则：插件签名时不可能知道用户的 NAS 地址，而 `credentials` 的 `hosts` 只能是可注册域名、不能是 IP（`docs/plugin-system.md` 4.2），装不下局域网的服务器。地址由用户在宿主画的表单里填，授权跟着这个来源走。

**串流不经过 JS**：tsx 的 `fetch` 把整个响应 `io.ReadAll` 成一个 `ArrayBuffer`（`tsx/web.go:701-711`，`Limits.Body` 默认 64 MiB，`tsx/sandbox.go:44-45`、`:56-57`），插件函数的返回值经 JSON（`core/host.go:18-20`），不能流式传二进制。所以 `Driver.Open` 由宿主做：先调插件的 `open` 拿到地址和头，再由宿主自己的 HTTP 客户端带 `Range: bytes=<offset>-` 去取，注入秘密的规则同上。重定向最多 5 次：同 origin 的保留所有头；跨 origin 的丢掉插件给的头和注入的秘密，并且必须是 https（Alist 常把下载重定向到网盘的 CDN）。回 200 而不是 206 时，宿主丢掉前 offset 个字节（服务器不支持 Range），并在日志里记一次。

### 11.4 权限、授权与审核提醒

- `kando.source.fetch` 属于「联网」一组（`plugins.GroupNetwork`）。安装确认和详情页的这一行写：「列出、读取你在这个插件的来源里填写的服务器上的文件。密码只由看懂发送到你填写的地址，插件看不到。」
- 因为 11.1 禁止来源插件有别的网络出口，它不构成 `docs/plugin-system.md` 6.2 的「读字幕 + 联网」高风险组合；来源插件也申请不了 `kando.lines.seen`（11.1，它用不到）。
- 残余风险如实写出：插件看得到用户 NAS 上的文件名和目录结构、看得到登录接口回来的会话令牌（令牌只能经 `kando.source.fetch` 发回同一台服务器）；恶意插件可以在这台服务器上做 `method` 允许范围内的事，比如 Alist 的 POST 接口能删文件。所以 `method` 白名单里没有 PUT、DELETE、MOVE、COPY、MKCOL，而 POST 只能发到清单里声明的路径前缀（`sources[].postPaths`，最多 8 个，例如 `/api/auth/login`、`/api/fs/list`、`/api/fs/get`），没声明的 POST 以 `denied` 拒绝。`kando-plugin preview` 在目录的 PR 评论里列出这些前缀给审核者看。
- 授权：安装后插件是停用的，打开时勾选 `kando.source.fetch`；用户之后在设置 › 媒体来源里添加这个种类的来源，填地址和密码，才真正有了可以访问的服务器。改地址时清掉秘密（3.1）。插件停用、卸载时，它的来源保留在设置里（见 3.1），浏览页显示「插件「X」已停用」，已导入的复制影片不受影响，索引影片不能播放。
- 离线模式（`docs/plugin-system.md` 6.2：离线时插件没有网络）：`kando.source.fetch` 在离线时只对局域网地址放行（12.4 的同一条规则），公网地址以 `unavailable` 拒绝。

### 11.5 官方插件

- `kando.webdav`（P4，参考插件）：`list` 用 `PROPFIND` 加 `Depth: 1`，解析 `multistatus`；`stat` 用 `Depth: 0`；`open` 直接回 `{path}`，认证由宿主的 Basic 注入完成。
- `kando.alist`（P6）：登录 `POST /api/auth/login`（`login: true`）拿令牌，`POST /api/fs/list`、`/api/fs/get`；`open` 回 `fs/get` 给的 `raw_url`（同 origin 的 `/d/…` 带签名，跨 origin 的经重定向规则）。
- `kando.jellyfin`、`kando.emby`（P6）：两者 API 同源，`X-Emby-Token` 头（`auth.header`），`list` 把「媒体库 → 剧集 → 季 → 集」映射成目录，`open` 回 `/Videos/<id>/stream?static=true`，只取原始文件、不让服务器转码。
- 源码放 `apps/kando/plugins/official/<名字>/`，和 `deepl/` 一样由 `make official-plugins` 签名、从 kando-plugins 发布（`docs/plugin-system.md` E14）。

## 12. 安全

### 12.1 已定事项（owner）

1. 新的插件扩展点「媒体来源」E17；SMB（SMB2/3）和 SFTP 内置，由宿主实现；WebDAV、Alist、Jellyfin、Emby 是官方脚本插件，经宿主的 `kando.net` 系 API 列目录和解析地址；第三方可以写新来源；内置和插件来源经同一个注册表出现。新依赖 `go-smb2` 和 `pkg/sftp` 已批准（SMB 库的选择见第 2 节第 1 条）。
2. 两种导入方式都做，每次导入由用户选。
3. 桌面和手机都做；手机能不经电脑直连 NAS。

### 12.2 资产与威胁

要保护的：来源的密码、私钥、会话令牌；用户 NAS 上的文件（看懂只读，不能被诱导去写或删）；文件名和目录结构（隐私）；本机的其他程序和网页不能借看懂读到 NAS 上的文件。

- **主机密钥（SFTP）**：首次连接时信任（TOFU），照存储位置的做法：没有可信密钥就不连，`HostKeyError` 带服务器给的密钥和 `SHA256:` 指纹（`storage/sftp.go:32-46`），对话框写指纹、建议和 `ssh-keygen -l` 的输出比对，用户点「信任」后存进来源的 `hostKey`（非秘密，存 settings.json）；之后密钥变了一律不连，弹 `AlertKeyChanged`，只给「取消」和破坏性的「替换密钥」。手机同样，手机和电脑各自信任，不同步密钥（不让一台被骗的设备带坏另一台）。
- **SMB 没有可验证的服务器身份**：NTLM 登录不认证服务器，`go-smb2` 也不暴露服务器 GUID（`cloudsoda/go-smb2@…/conn.go:154` 那一行是注释掉的），所以 SMB 做不了 TOFU。如实写出两条：一是冒充的服务器能拿到 NTLMv2 的应答，可以离线猜密码，所以来源表单在 SMB 下面写「只在你信任的网络里使用，不要和别的账号共用这个密码」；二是本设计建议默认开启消息签名（`smb2.Negotiator.RequireMessageSigning`，`conn.go:20-24`；现在的 `dialSMB` 没有设，`storage/smb.go:89`），签名用的会话密钥由密码导出，不知道密码的中间人改不了内容。有些老 NAS 不支持签名，默认值和例外见第 17 节第 4 条。
- **HTTP 来源**：11.3 的 http 规则；https 来源校验证书，自签名证书在 P4 起按证书指纹首次信任，规则和 SFTP 相同（第 17 节第 5 条）。
- **回环服务器**：7.2。残余风险：同用户的程序可以读看懂的内存或抓它给播放器的 URL，这本来就在 `docs/plugin-system.md` 6.1 的 T7 范围之外。
- **路径**：所有来源路径都过 `cleanPath`（`storage/storage.go:60-81`）；插件 `list` 回的名字按 11.2 的规则过滤，过滤不过的条目丢掉，不让插件用 `../` 把看懂引到根目录外，也不让它造出能写到资料库外面的占位路径（6.3 的占位路径再过一次 `filepath.IsLocal`）。
- **写**：媒体来源的驱动接口里没有写方法（第 3 节），内置驱动包 `storage.Provider` 时不暴露 `Write`、`Remove`、`Move`、`MkdirAll`，用类型保证看懂不会改用户 NAS 上的文件。

### 12.3 隐私文案

设置 › 隐私加一段「媒体来源」（`privacy…` 前缀的文案，见第 14 节）：

- 「看懂只读取你的媒体来源，从不在上面写入或删除文件。」
- 「来源的密码保存在系统的钥匙串（Windows 是凭据管理器，Linux 是 Secret Service）里，不写进看懂的设置文件，插件也看不到。」
- 「只建索引的影片，播放和做字幕时从来源读取；做字幕时大约读一遍整个文件，只临时保存当前一段的声音。」
- 「发给手机的只有影片和字幕，NAS 的地址和你的密码不会发给手机，也不会写进反馈。」

### 12.4 离线模式

离线模式现在的含义是「关掉互联网」：对象存储被拒，NAS 上的存储位置照常（`playback_stored.go:26-30`：「Offline mode keeps the internet, and with it an object store, closed」）。媒体来源沿用这个含义：主机是回环、私有（RFC 1918、`fc00::/7`）、链路本地地址，或者名字解析到这些地址（`.local`、`.lan` 和局域网 DNS 给的名字，**按解析结果判断，不按名字判断**），算局域网，离线模式下照常可用；其余算互联网，离线时内置来源不连、插件来源的 `kando.source.fetch` 拒绝、索引影片的播放和任务以 `ErrOffline` 失败并指向隐私设置。判断放在非界面包 `mediasource.Local(ctx, host)`，内置驱动拨号前和 `kando.source.fetch` 发请求前都调它，所以名字解析到公网地址时不会被当成局域网。Tailscale 之类的 `100.64.0.0/10` 不算局域网（第 17 节第 8 条）。

## 13. NSGo 的缺口

已查重并提交：第 1 条是 [nsgo-dev/nsgo#1061](https://github.com/nsgo-dev/nsgo/issues/1061)（已由 nsgo PR #1153 修好：`android.loopback_cleartext`），第 2 条补在 [nsgo-dev/nsgo#681](https://github.com/nsgo-dev/nsgo/issues/681#issuecomment-7054165773) 下，第 3 条按第 17 节第 9 条留到 P5。

1. **（已修好，nsgo#1061）Android 不能允许 app 访问自己的回环 HTTP**：清单模板的 `<application>` 没有 `android:usesCleartextTraffic`，也没有 `android:networkSecurityConfig`（`internal/platform/android/runtime/AndroidManifest.xml:29-33`），`apk.Manifest` 没有对应字段（`cmd/nsgo/apk/axml.go:17-48`），`nsgo.yaml` 的 `android` 只有 `permissions`、`target_sdk`、`recording`（`cmd/nsgo/project.go:171-175`）。targetSdk ≥ 28 时系统默认拒绝明文，`core.Video` 在 Android 上用 `MediaPlayer.setDataSource(context, Uri)`（`VideoNode.java:219-242`），读不了 app 自己在 `127.0.0.1` 上的串流服务器。建议：`nsgo.yaml` 加一个只放开回环的选项（生成 `res/xml/network_security_config.xml`，`<domain-config cleartextTrafficPermitted="true">` 只含 `127.0.0.1` 和 `localhost`）。查重时 `gh issue list -R nsgo-dev/nsgo --state all --search cleartext` 和 `networkSecurityConfig` 都没有结果。影响：手机直连在 Android 上没有串流，只能下载后播放（10.3）。上游 `docs/architecture/updates.md:74` 已经提到「Android refuses cleartext」，可以在 issue 里引用。
2. **`core.Video` 不能由 app 提供数据**：`driver.VideoConfig` 只有 `Source string`（`internal/driver/mount.go:1448-1461`），没有请求头、reader 或 resource loader（AVFoundation 的 `AVAssetResourceLoaderDelegate`、Media Foundation 的 `IMFByteStream`、Android 的 `MediaDataSource` 都能做到）。请求头的部分已有 [nsgo-dev/nsgo#681](https://github.com/nsgo-dev/nsgo/issues/681)（OPEN）；本设计的新场景是「app 用 Go 读的字节流（SMB、SFTP）」，适合在 #681 下补一条评论，提议一个 `Source` 之外的 `Open func(offset int64) (io.ReadCloser, error)` 加 `Size`，而不是另开 issue。看懂不等它：回环服务器在桌面和 iOS 上都能用。
3. **没有局域网发现**：NSGo 没有 mDNS / Bonjour / NSD 的 API，`core.Permission` 里也没有本地网络这一项（`core/permissions.go:14-42`），固定版本里搜 `NWBrowser`、`NsdManager`、`zeroconf`、`avahi` 都没有。这不挡本设计（第一版由用户填地址），只是体验上的缺口；要不要提由 owner 定（第 17 节第 9 条）。

iOS 的 ATS 不算缺口：`apple.info` 能写嵌套字典（`cmd/nsgo/plistvalue.go:41-58`、`plistvalue_test.go:44`），在 `apps/mobile/nsgo.yaml` 里加就行（10.3）。

## 14. 多语言

五份 catalog 都要有（`apps/kando/locales/` 和 `apps/mobile/locales/`），`en.yaml` 是源语言，带 `description` 和 `placeholders`；文字在绑定里取，跟着 `cx.SetLocale` 变（`AGENTS.md`）。`source…` 前缀已经被侧栏的来源筛选占用（`sourceAll`、`sourceLocal`，`locales/en.yaml:6395-6417`），所以本块用 `mediaSource…`：

- 侧栏与浏览：`mediaSourceSidebar`（「媒体来源」）、`mediaSourceRoot`、`mediaSourceShares`、`mediaSourceEmpty`、`mediaSourceListing`、`mediaSourceHasSubtitles`、`mediaSourceInLibrary`、`mediaSourceImportSome`（`{count, plural, …}`，「导入 N 个」）、`mediaSourceImportFolder`、`mediaSourceTooMany`（`{limit}`）。
- 导入对话框：`mediaSourceImportTitle`（`{count, plural}`）、`mediaSourceImportCopy`、`mediaSourceImportCopyNote`（`{size}`）、`mediaSourceImportLink`、`mediaSourceImportLinkNote`（`{source}`）、`mediaSourceImportRemember`、`mediaSourceImportStart`。
- 状态与错误：`mediaSourceUnreachable`（`{source}`）、`mediaSourceGone`、`mediaSourceRemovedMark`、`mediaSourceChanged`（`{source}`）、`mediaSourceChangedRedo`、`mediaSourceChangedKeep`、`mediaSourceMissing`（`{source}`）、`mediaSourceFind`、`mediaSourcePluginOff`（`{plugin}`）、`mediaSourceOffline`、`mediaSourceNoHTTP`（ffmpeg 不能读网络地址）、`mediaSourceCopyToLibrary`、`mediaSourceFrom`（卡片上「来自 {source}」）。
- 设置：`settingsSectionMediaSources`（节名）、`settingsSourcesAdd`、`settingsSourcesKind`、`settingsSourcesKindSMB`、`settingsSourcesKindSFTP`、`settingsSourcesPlugin`（`{plugin}`，「插件 · X」）、`settingsSourcesFromLocation`、`settingsSourcesPlainHTTP`、`settingsSourcesSMBWarning`、`settingsSourcesNoSecretStore`、`settingsSourcesRemoveConfirm`（`{count, plural}`）。SFTP 的信任和密钥变了的文案复用存储位置已有的 `accountLocation…` 条目，不复制一份。
- 隐私：`privacySources`、`privacySourcesReadOnly`、`privacySourcesSecrets`、`privacySourcesStream`、`privacySourcesPhone`（12.3 的四句）。
- 插件：`pluginsPermSourceFetch`、`pluginsPermSourceFetchNote`、`pluginsContribSource`（`{title}`）、`pluginsErrSourceNetwork`。
- 手机（`apps/mobile/locales/`）：`mediaSourceSection`、`mediaSourceAdd`、`mediaSourceDownload`、`mediaSourceRemoveDownload`、`mediaSourceStreamUnavailable`（Android 上没有串流时的说明）、`mediaSourceSubtitlesFromComputer`（`{computer}`）、`mediaSourceMakeOnComputer`、`mediaSourceHomeOnly`（只在家里的网络可下载），以及 InfoPlist.strings 的 `NSLocalNetworkUsageDescription` 改写（4.2）。
- 来源的种类名（SMB、SFTP、WebDAV）是专有名词，各语言相同，但仍各写一条，`catalog_test.go` 要求五份齐全。存进 settings.json 的种类 ID、字段键和 `kando-source://` 地址是数据，不翻。

## 15. 代码布局

### 15.1 非界面包（不导入任何界面包）

- `apps/kando/mediasource/`：`Kind`、`Field`、`Source`、`Driver`、`Entry`、注册表（`Registry`，引擎 goroutine 上读写，驱动在后台用）、来源地址的 `Ref`（`ParseRef`、`String`）、`Local(ctx, host)`（12.4）、`SourceKey`（10.5）、占位路径的计算（`Placeholder`，6.3）、按来源 ID 持有连接的 `Pool`、复制导入的 `Copier` 和旁边字幕的 `SubtitlesOf`（5.1）、内置种类的驱动（`smb.go`）。测试跟着进包。
- `packages/remotefs/`（P5，从 `apps/kando/storage` 挪出来，10.1）。在 P5 之前，内置驱动直接包 `apps/kando/storage`。
- `packages/core`：`PlatformSource` 和 `ParseLink` 认 `kando-source://`；`core.Job.Linked`。
- `packages/media`：`SourceReader`、`Tools.Sources`、`Download` 和 `Make` 的两处分支（第 8 节）。
- `packages/sync`：`psync.Film.SourceKey`（可选）。
- `apps/kando/plugins`：`Source`（清单的 `contributes.sources`）、`PermSourceFetch`、`ErrSourceNetwork`、`Validate` 的规则（11.1）。

### 15.2 界面目录

- `apps/kando/sourcebrowser/`：浏览页（4.1），读自己的 `App` 接口（嵌入 `live.State`），图标放 `icons.go`。
- `apps/kando/settings/mediasources.go`：设置 › 媒体来源，照 `Window.Plugins` 的先例由 `settings.Window.MediaSources` 注入（读自己的 `settings.MediaSourcesApp`），不往 `settings.App` 里加方法。
- `apps/mobile/sources/`：手机的来源列表和目录页（4.2）。

### 15.3 根目录 package main 的新文件

- `state_mediasource.go`：`mediaSourceUI`（来源列表、Pool 里的驱动、浏览状态、导入对话框的标志）、`savedMediaSources` 和 `clean()`、`mediaSourceHooks()`（`restore` 读设置；`quitStarting` 停掉正在列的目录；`quitting` 关驱动；插件块变化后重建插件种类）、`mediaSourceKeys(cx)`（「浏览媒体来源」一行，`Key` 留空）、`mediaSourceOverlay(cx)`（导入对话框、删除来源的确认）。
- `state_mediasource_browse.go`：浏览页和导入对话框的状态，`mediaSourceCopier` 把 `mediasource.Copier` 交给 `Tools.Sources`（P1）。
- `state_mediasource_import.go`：`mediaSourceLink`、`Tools.Sources.URL` 的实现、文件变了的检查（P2）。
- `playback_linked.go`：`linkedOf` 和播放的接法（7.3），挨着 `playback_stored.go`。
- `bind_mediasource.go`：`sourcebrowser.App` 和 `settings.MediaSourcesApp` 的 getter 和 action（`live.Func`、`live.Do`）。
- `state_plugins_sources.go`：插件种类的注册和注销、插件驱动（调 `list`、`stat`、`open`）、`kando.source.fetch` handler（放在插件块里，和 `credentialFetch` 挨着）。
- 测试：`mediasource_test.go`、`screenshot_mediasource_test.go`。

### 15.4 共享文件（各一行）

- `main.go`：`app` 加字段 `sources mediaSourceUI`，`newApp` 加一行。
- `settings_file.go`：`savedSettings` 嵌入 `savedMediaSources`，`clean` 加一行。
- `events.go`：`areas()` 加 `a.mediaSourceHooks()`。
- `keys.go`：`shortcuts()` 加 `a.mediaSourceKeys(cx)`。
- `state_overlay.go`：`overlays()` 加 `a.mediaSourceOverlay(cx)`。
- 另外 `state_settings.go` 的 `settingsWindow` 加 `MediaSources: settings.MediaSources{App: a}`，`settings/sections.go` 的 `sections` 加一行，`settings/sectionid` 加 `MediaSources`。

### 15.5 各接点的最小改动

- `storage/proxy.go`、`pool.go`、`transfer.go`：`Provider` 参数放宽成 `Reader`；`Proxy` 加 `Host` 检查和 token 撤销（P0）。
- `playback_stored.go` 的入口、`state_phonesync.go` 的 `phoneFilmOf`、`films.MovedVideos`：在 `Stored` 之后再查 `Linked`（P2）。
- `queue.go` 的 `queuePipeline`：给 `Tools.Sources` 赋值；`state_privacy.go` 的 `privacyRefusesLink`：认 `PlatformSource`（P1）。
- `sidebar/sidebar.go`：「媒体来源」一行（P1）。
- `state_feedback.go` 的 `feedbackSecrets`：加来源的秘密（P1）。
- `locales/*.yaml` 五份和 `catalog_gen.go`；手机的五份和 InfoPlist.strings。
- 不改：`media.Pipeline`、`Run`；存储位置的设置和 `stored`；`phonesync` 的路由和协议版本。

## 16. 新依赖

桌面在 P0–P4 不新增任何 module，用到的都已经在 `apps/kando/go.mod` 里：

| 模块 | 版本 | 许可证 | 用途 | 位置 |
|---|---|---|---|---|
| `github.com/cloudsoda/go-smb2`（第 12 行） | `v0.0.0-20261105190815-95be04a983b8`（2026-11-05，已是最新） | BSD-2-Clause | SMB 2/3 客户端，内置 SMB 来源；`ListSharenames` 列共享 | `apps/kando/storage`，P5 起 `packages/remotefs` |
| `github.com/pkg/sftp`（第 15 行） | `v1.13.12`（2026-10-23，已是最新） | BSD-2-Clause | SFTP 客户端，内置 SFTP 来源 | 同上 |
| `golang.org/x/crypto`（第 20 行） | `v0.59.0` | BSD-3-Clause | `ssh`：SFTP 的传输、主机密钥和指纹 | 同上 |
| `github.com/studio-b12/gowebdav`（第 16 行） | `v0.13.0` | BSD-3-Clause | 本设计不用（WebDAV 走插件），存储位置照旧用 | `apps/kando/storage` |
| `golang.org/x/net/webdav`（`golang.org/x/net v0.62.0`，第 22 行） | 同 | BSD-3-Clause | 只在测试里起假 WebDAV 服务器（`storage/webdav_test.go:13` 已在用） | 测试 |

owner 批准的 `github.com/hirochachacha/go-smb2` 最新是 `v1.1.0`（2022-03-12，BSD-2-Clause），本设计建议**不加**，理由见第 2 节第 1 条。

P5 新增的是**已有依赖进入新的 module**：`packages/remotefs/go.mod` 和 `apps/mobile/go.mod` 会 require 上面的 go-smb2、pkg/sftp、x/crypto（版本同上，`go.work` 下统一），手机的二进制因此变大，P5 第一个 PR 量出 iOS 和 Android 包的增量报给 owner。`cloudsoda/go-smb2` 还会带进它的依赖 `github.com/jcmturner/gokrb5/v8 v8.4.4`、`github.com/geoffgarside/ber v1.1.0`、`github.com/cloudsoda/sddl`（`cloudsoda/go-smb2@…/go.mod`），桌面已经间接有了，手机是新的。

## 17. owner 的决定（2026-11-21）

1. **SMB 库**：用已经在 go.mod 里的 `cloudsoda/go-smb2`，不加 `hirochachacha/go-smb2`（理由见第 2 节第 1 条）。
2. **存储位置的秘密**：迁出 settings.json，单独一个 PR，和媒体来源同样放进 `core.Secrets`；Linux 没有 Secret Service 时保留明文退路，设置里写明。
3. **WebDAV**：内置。P3 之后加一个小 PR 把 `storage/webdav.go` 注册成内置种类，手机也能直连 WebDAV；P4 的官方 WebDAV 插件照做，作为脚本来源 API 的参考插件，两者同时存在时内置的排在前面。
4. **SMB 消息签名**：默认要求，表单里给「这台 NAS 不支持签名」的开关，打开时写明风险。
5. **HTTPS 来源的自签名证书**：做证书指纹的首次信任，放进 P4，规则和 SFTP 相同。
6. **索引影片经存储中转给外出的手机**：先不做，手机上标「只在家里的网络可下载」。
7. **手机请电脑给 NAS 上的文件做字幕**：放在 P5 的最后，可以砍掉。
8. **离线模式里的局域网**：只算回环、RFC 1918、ULA、链路本地；Tailscale 等 CGNAT 段（`100.64.0.0/10`）算互联网。
9. **局域网发现的 NSGo issue**：不挡本设计，P5 做手机直连时再提。
10. **导入方式的默认值**：默认「复制到资料库」，用户勾「记住我的选择」后按记住的。

## 18. 风险

1. **串流时 ffmpeg 读完整个文件**：做一次字幕约从 NAS 读一遍整个视频（第 8 节），无线网络上的大文件可能比复制还慢。缓解：导入对话框写明；任务的进度照常显示，可以取消。
2. **NAS 休眠**：很多 NAS 硬盘休眠后第一次读要等 10–20 s，`PoolOpenTimeout` 是 30 s，够用；播放器的首帧会慢，播放页在打开阶段显示「正在连接 <来源名>」。
3. **SMB 冒充服务器**（12.2）：没有服务器身份，只能靠文案和签名缓解。
4. **旧版看懂丢掉 `linked`**（6.2）：降级后索引影片只剩字幕，和 `stored` 一样；写进发版说明。
5. **Android 没有串流**（第 13 节第 1 条）：上游修好之前，手机直连在 Android 上只能下载后播放。
6. **插件来源的 POST**：11.4 的路径前缀白名单限制不了同一个接口里的动作（和 AnkiConnect 同类问题，`docs/plugin-system.md` 4.2 末尾），靠签名、审核和最窄的前缀。
7. **文件名编码**：SMB 和 SFTP 的文件名一般是 UTF-8，老 NAS 可能是本地编码。不是合法 UTF-8 的名字在列表里用替换字符显示、不能导入（路径会对不回去），提示用户在 NAS 上改名。
8. **P5 的 `storage` 拆包**会碰到 `apps/kando/storage` 的全部文件和存储位置、手机同步的导入，超过 10 个文件，按全局规则先把计划给 owner 看。

## 19. 测试

- **`mediasource` 单元测试**：来源地址的 `Parse`/`String` 往返（含空格、中文、`%`、`#`）；路径越界拒收；占位路径的替换、重名序号和 `filepath.IsLocal`；`Local` 对回环、RFC 1918、ULA、链路本地、公网和 `100.64.0.0/10` 的判断，名字按解析结果判断（测试注入解析函数）；`SourceKey` 稳定、大小写不同的主机名相同、不同主机不同。
- **驱动的接缝**：`mediasource.Driver` 本身就是接缝。app 级测试用内存里的假驱动（`fakeSource`：一棵固定的目录树，`Open` 读 `writeFakeClip` 写的占位文件，可以设成「连不上」「文件变了」「慢」），注册成一个测试种类，不连网。`mediaSourceUI` 照 `locationsUI.open` 的先例持有一个可替换的 `open` 函数（`state_locations.go:32`），测试换成返回假驱动。
- **SFTP**：沿用 `storage/sftp_test.go` 的 `fakeSSH`（回环上的真 SFTP 服务器，ed25519 主机密钥），测首次信任、密钥变了拒连、Range 读、断线重连。
- **SMB**：Go 里没有 SMB 服务器（`storage/smb_test.go:14`），单元测试只测到接缝为止；真协议的测试沿用 `KANDO_TEST_SMB`（例如一个 Samba 容器），CI 里没设就跳过。要不要在 CI 里起 Samba 容器跑这一组，P1 时看 CI 的 Linux runner 能不能跑 Docker，能就加一个只跑 `TestSMB` 的 job。
- **回环串流服务器**：Range、If-Range、HEAD、多段 Range；错 token、空 token 404；`Host` 头不对 421；撤销后 404；只听 127.0.0.1（断言监听地址）；没有 CORS 头；来源连不上时 502。
- **流水线**：`packages/media` 的假工具（`fake_tools_test.go`）里，`Linked` 任务的 `Hear`、`Length` 拿到的输入是 `Tools.Sources.URL` 给的地址而不是占位路径；复制方式的进度经 `Trace.Progress` 报出；离线时公网来源以 `ErrOffline` 失败、局域网来源照常；ffmpeg 不支持 http 时 `Check` 失败。
- **library.json**：`linked` 的读写往返；旧文件没有 `linked` 照常读；删除索引影片不调驱动的任何写方法（假驱动对写调用直接让测试失败）。
- **插件来源**：`plugins` 的 `Validate`：声明 `sources` 却有 `network`、`credentials`、`llm.chat`、`lines.seen` 的清单拒收；字段规则。app 层用假脚本插件（`docs/plugin-system.md` 第 9 节的夹具做法）加 `httptest` 起的假 WebDAV 服务器（`golang.org/x/net/webdav`，`storage/webdav_test.go:13` 已在用）：`kando.source.fetch` 越出 `endpoint` 路径、方法不在白名单、POST 到没声明的前缀都 `denied`；Basic 注入正确且插件读不到；跨 origin 重定向丢掉秘密；插件 `open` 回 200 时宿主丢掉前 offset 字节；改地址后秘密失效。
- **手机**：`apps/mobile` 的测试用同一个假驱动接口测「下载到手机」的续传、`SourceKey` 匹配电脑的影片后取电脑的字幕；Android 上不显示串流按钮。
- **同步协议**：`TestSourceKeyIsAnAddition`（9 节）；`phonesync` 测索引影片经 `Remote` 发出、来源已删除时按只有字幕发出（照 `phonesync/remote_test.go:32` 的 `TestServesAVideoMovedToAStorageLocation`）。
- **截图场景**（`screenshot_mediasource_test.go` 的 `TestScreenshotsMediaSources`，假驱动和样例库，路径在 `f.home` 下显示成 `~/…`，主机名是场景里写的名字、从不连接）：`media-sources-browse`（来源列表加一个目录，含「有字幕」和「已在资料库」）、`media-sources-shares`（SMB 的共享列表）、`media-sources-import`（导入对话框）、`media-sources-unreachable`（播放页「连不上」）、`media-sources-changed`（文件变了的提示）、`settings-sources`（设置页，一个内置一个插件来源）、`settings-sources-add`（添加 SMB 的表单，含签名开关和警示）。SFTP 的信任对话框复用 `settings-account-location-trust` 的组件，不另截。手机：`TestScreenshotsSources`（`apps/mobile`）的来源列表和目录。

## 20. 分阶段落地计划

每一期都能单独合并和发布，前一期不依赖后一期。改动超过 10 个文件的 PR 先把计划给 owner 看。

- **P0 注册表和回环串流服务器（用户看不到变化）**
  - PR 0.1：`storage` 的 `Reader` 接口，`Handler`、`Proxy`、`Get` 改收它；`Proxy` 加 `Host` 检查和 token 撤销，播放器换影片时撤销（存储位置的播放从这里就受益）。加 7.2 的测试。
  - PR 0.2：`apps/kando/mediasource`：`Kind`、`Driver`、`Entry`、注册表、来源地址、`Local`、占位路径；`packages/core` 的 `PlatformSource` 和 `core.Job.Linked`。只有单元测试，app 还不注册任何种类。
- **P1 桌面上 SMB 的复制导入**
  - PR 1.1：设置 › 媒体来源（只有 SMB 一种）、`savedMediaSources`、秘密进 `core.Secrets`、`feedbackSecrets`、列共享、五份 catalog、截图 `settings-sources`、`settings-sources-add`。
  - PR 1.2：浏览页 `sourcebrowser/`、侧栏一行、导入对话框（这一期只有复制一种方式，对话框不显示方式的单选项，P2 再加）、`Tools.Sources.Copy`、离线规则、截图 `media-sources-browse`、`media-sources-shares`、`media-sources-import`。
- **P2 串流（只建索引）**
  - PR 2.1：library.json 的 `linked`、占位路径、`films.MovedVideos` 列出、`playback_linked.go`、`phoneFilmOf` 的分支、来源删除和重新加回、文件变了的检查；导入对话框加上「只建索引」。
  - PR 2.2：`packages/media` 的 `Make` 认 `Linked`、ffmpeg 读回环地址、ffmpeg 的 http 检查、重新处理和导出声音；「复制到资料库」菜单项。截图 `media-sources-unreachable`、`media-sources-changed`。
- **P3 SFTP**：注册 SFTP 种类，私钥和口令字段，首次信任和密钥变了的对话框（复用 `account` 的组件），`fakeSSH` 的测试。
- **P4 脚本来源 API 和官方 WebDAV 插件**
  - PR 4.1：`plugins` 的 `contributes.sources`、`PermSourceFetch`、`Validate` 的规则（11.1）、详情页和安装确认的文案。
  - PR 4.2：插件种类的注册、插件驱动、`kando.source.fetch`、串流时宿主的取法和重定向规则；假 WebDAV 服务器的测试。
  - PR 4.3：`apps/kando/plugins/official/webdav/`，签名发布。
- **P5 手机直连**
  - PR 5.0：把协议代码挪进 `packages/remotefs`（行为不变，先给 owner 看计划），量出手机包的体积增量。
  - PR 5.1：手机的来源设置、`sources.json` 和秘密、目录页、下载到手机、iOS 的 ATS 和本地网络文案；Android 只有下载（等第 13 节第 1 条）。
  - PR 5.2：iOS 上的串流（手机的回环服务器）。
  - PR 5.3：`psync.Film.SourceKey`、手机按它取电脑的字幕；（可选）请电脑做字幕（第 17 节第 7 条）。
- **P6 Alist、Jellyfin、Emby**：三个官方插件各一个 PR（`kando.alist`、`kando.jellyfin`、`kando.emby`），用 P4 的 API，`login` 注入和 `postPaths` 在这里第一次真正用上；每个带自己的假服务器测试（`httptest` 写最小的几个接口）。

验收：在只开了一台 Samba 容器和一台 SFTP 服务器的局域网里，桌面能浏览、复制导入、索引导入并播放、给索引影片做字幕、断开 NAS 后看到正确的提示；手机能经电脑下载索引影片，也能直连 NAS 下载（iOS 还能串流）；装了官方 WebDAV 插件后，WebDAV 来源和 SMB 来源在浏览页上看起来一样。
