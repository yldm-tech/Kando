# Kando 插件系统设计

> 本设计以 minimal-fit 方案为底，按学习价值排序的扩展点取自 user-value 方案，信任模型取自 security 方案。第一轮评审指出的致命问题已经改掉：授权不再放在 settings.json 里，控制 socket 不能用来安装，插件的贡献声明纳入签名，第一期就交付离线词典和逐词标注，外部原生工具这一层取消。第二轮评审的阻断和主要问题按条改进正文，处理结果列在第 13 节「评审记录」。文中的行号都在当前 main（57b9e6cb）上重新核对过。NSGo 的 API 以固定版本 `v0.1.1-0.20261118094851-ec272782be28` 的源码为准。本轮的范围和已定事项见 12.2，仍待决定的见 12.3，各 PR 的进度见第 15 节。

## 1. 目标与非目标

### 目标

- 不发新版本，也能让用户和第三方给看懂加内容。第一批是学语言的人最常用的几类：离线词典、词表（JLPT/HSK/CEFR 等级和词频）、术语表、AI 服务预设、翻译风格。之后再开放能执行逻辑的扩展：命令、查词来源、字幕导出格式、学习导出目标、逐词标注和候选译文来源。
- 整个插件系统作为一个新的块接入，遵守 `apps/kando/AGENTS.md`。纯逻辑放在非界面子包里，每个共享文件只加一行。插件状态经 `live` 层读，后台工作经 `a.update` 回到引擎 goroutine。
- 能执行的插件只用固定版本 NSGo（`v0.1.1-0.20261118094851-ec272782be28`）自带的 `nsgo/tsx` 和 `nsgo/tsx/pkg`。不自己造 JS 引擎、沙箱、签名或吊销机制，也不新增 module。
- 插件永远碰不到这些东西：settings.json、library.json、用户的 LLM key、浏览器 cookie、绝对路径，以及还没播到的字幕。
- macOS、Windows、Linux 三个平台行为一致。安全上做不到一致的地方（凭据库对同用户是否开放）在 6.7 节按平台写明。

### 非目标

- 插件不能替换流水线阶段（下载、识别、校对、翻译、导出落盘），也不能在 `media.Run` 中间挂通用钩子。原因是 `Run` 的续跑和重做依赖文件语义（转写稿、`pendingHeard`、checkpoint、proofs），而 `apps/kando` 里有大量 `a.pipeline.(media.Tools)` 断言（非测试文件约 73 处）。`packages/media` 只做两处加法：P2.0 的 `ChatWith`（见 5.3），以及第 5 期可选的 `media.Tools.Engine` 字段（见 2.3）。两处都不改 Pipeline、Run 的语义，也不碰任何断言。
- 不支持 Go `plugin`、原生 dylib、第三方原生 sidecar 和 MCP 子进程插件，原因见 3.4 节。要不要开放 MCP 子进程，以后单独立项、由 owner 决定，不写进本路线图。
- 插件不能自己画整窗覆盖层，不能在影片画面或字幕层上自绘，也不能注册顶层 CLI 命令或 socket 方法。
- 手机端不运行插件代码。
- 第一版不做社区索引和在线商店。

## 2. 扩展点清单（按优先级）

优先级综合三项：对学习者的价值、实现成本、对现有语义的风险。

### 2.1 数据包扩展点（不执行代码）

**E1 离线词典（P1，价值最高）**
- 桌面和手机的查词现在是两条独立的路：桌面走 `apps/kando/state_vocab.go:215` 的 `func (a *app) LookUpWord(i int, word string)`，它是界面动作，会改 `a.vocab.lookup` 并暂停播放；手机走 `state_phonesync_study.go:65-105` 的 `phoneStudy.LookUp`，自己的流程是 `vocabCache().Get(q)` → `s.ask(f)` → `got.ask.t.Chat(...)`，不经过 `LookUpWord`，没配 AI 时在 `s.ask(f)` 就失败。
- 所以先抽出一个共用的 `func (a *app) vocabResolve(ctx, q) (lookup.Entry, vocabSource, bool)`，顺序是缓存 → 已启用的词典（按 `savedPlugins` 里的顺序）→ AI。`LookUpWord` 和 `phoneStudy.LookUp` 都改成调它，并且词典要放在 `s.ask(f)` 之前，离线的手机也能查到词。手机只读（`psync.Caller.ReadOnly`）时的规则不变：词典结果不写进缓存。
- 日语查词要用原形，但现有分词给不了原形和读音：`packages/core/segment/segment.go:174-176` 有意用 `ipa.DictShrink()` 加载了去掉 features 的 IPA 词典，只保留词界和词性，以节省内存，手机也受益于此；`:191` 把助动词并进前一个词（`行った` 是一个词，没有单一的 kagome 原形）；中文的 gse（`:203-210`）只返回字符串，没有读音。因此原形和读音要单独做（P1.2）：
  - 新增 `segment.Annotated(lang, line) []Morph{Text, Word, Base, Reading}`，内部用另一个按需加载的完整 features 词典 `ipa.Dict()`，只在桌面上、并且启用了日语的词典包或词表包时才加载（12.2 第 5 条的测量门槛达标的前提下）。`Words` 保持用 `DictShrink`，`core.WordToken` 不改，手机的分词不受影响。合并后的词（`行った`）取它首个 token 的原形（`行く`）。
  - P1.2 先在 Ubuntu 编译机（`ubuild`）上量出 `ipa.Dict()` 相对 `DictShrink()` 的常驻内存增量（RSS）和首次加载时间。门槛是增量不超过 200 MiB、加载不超过 3 s（12.2 第 5 条）。不达标就退到另一条路：Annotated 不出 Base 和 Reading，查词先查表层形，查不到再查拆开后首个 token 的表层形，读音只取自词典包的条目。测量数字和最终选择记在第 15 节。
  - 中文的读音（拼音）只来自 CC-CEDICT 这类词典包，gse 不提供读音。
- 验收：装了对应的词典包后，断网、没配 AI 时，在桌面和手机上点日语或中文的词，都能看到释义。

**E2 词表与逐词标注（P1）**
- 接入位置：`apps/kando/film/subtitle.go:269` 的 `type lineWords struct`（在 `:213` 构造）。给它加一个 `marks func() []subtitle.Mark`，由 `withWords` 合成 `core.Span` 的 `Style`（语义色）或 `Attachment`（ruby 注音）。
- `Mark` 定义在叶子包 `film/subtitle`，所以 `film` 不导入插件包。
- 数据来源：读音来自 P1.2 的 `segment.Annotated`，或者词典包里的读音和拼音（P1.2 不达标时 ruby 只能用词典包）；等级和词频来自 wordlist 包；已学词来自 study 的 WordIndex。
- 何时计算：按需计算，以「视频 + 字幕内容哈希」为键。触发点是插件块自己的 `playing` 钩子：播放器每次报告进度时都会调到它，它只比较当前影片的键和缓存的键，键不同时才起后台计算，算好经 `a.update` 存成不可变切片，再返回 changed。不放在 `shelfChanged` 里算，因为每次任务报进度都会触发它。也不去 film 块的打开函数里插调用，AGENTS.md 不允许这样做。绑定里只读缓存，从不调用插件。

**E3 术语表包（P1）**
- 接入位置：`apps/kando/state_glossary.go:119` 的 `return job, core.MergeTerms(own, series, global)`，改成 `core.MergeTerms(own, series, append(global, a.pluginTerms(job)...))`，按包声明的 from/to 和任务语言过滤。
- `MergeTerms` 按 `SameTerm` 去重、先到的留下，所以用户自己的条目和 Locked 条目总是优先。
- `:151` 的 `func (a *app) FilmTerms()`（播放时用的 `core.MergeTerms(a.gloss.film, series, global)`）同样追加一份。
- 每个包最多 5000 条。

**E4 AI 服务预设（P1）**
- 现在服务商的身份只由包级函数决定，这两个函数只认内置预设：`settings/presets.go:72` 的 `PresetByID(m, id)` 只遍历 `Presets(m)`，`:82-92` 的 `ProviderID(base)` 只遍历 `Presets(locales.In(locales.Source))`，认不出就返回 `"custom"`。所以只在调用方追加插件预设没有用：`settings_ai.go:123` 会把插件的地址认成 `custom`，`:141` 的 `selectLLM` 会拒绝 `pack.` id，`:174` 的 `llmRemember` 会把服务存到 `custom` 下。
- 改法：预设解析要知道当前 app 有哪些预设。`PresetByID(list []Preset, id)` 和 `ProviderID(list []Preset, base)` 都改成接收预设列表，app 提供 `a.llmPresets()`，内容是 `settings.Presets(m)` 加上 `a.PluginPresets()`。所有调用处一起改（不能只改一半）。`PresetByID` 的调用处：`settings_ai.go:47,141,215,392`、`bind_onboarding.go:92`、`bind_settings.go:186`、`state_feedback.go:184`、`state_phonesync_settings.go:57`；`ProviderID` 的调用处：`settings_ai.go:123,174`、`bind_settings.go:195`、`state_feedback.go:184`、`state_phonesync_settings.go:57`。`settings_ai.go:47` 在 `savedLLM.clean()` 里，那里没有 app，只传 `settings.Presets(...)`，插件服务靠 P0.2 的 `pack.` 前缀保住。另有三处直接读 `settings.Presets` 的列表，按用途分开处理：`bind_settings.go:189` 的 `Presets()` 是设置 › AI 下拉框（`settings/ai.go:155`）的数据源，P1.1 改成 `a.llmPresets()`；`bind_onboarding.go:119` 的引导页服务列表和 `state_feedback.go:367` 的 `feedbackPublicEndpoint` 只认内置预设，不改。这一步作为 P0.1 单独成 PR，行为不变，涉及约 13 处、6 个文件。
- 插件预设的 id 统一用 `pack.<id>`，只预填 BaseURL、Model、KeyURL、Group。key 和 host 的绑定规则不变（`SameHost`、`restoreLLM`）。
- 必须同时修一处：`settings_ai.go:40` 的 `savedLLM.clean()` 在 `:47-48` 遇到未知 preset id 时会 `delete(s.LLMServices, id)`，插件暂时缺席（比如所在的盘没挂上）时会把用户的 key 丢掉。改成保留 `pack.` 前缀的服务。测试要先在插件预设在场时让服务真的存到 `pack.<id>` 下，再拿掉插件、跑 clean，断言 key 还在。
- 防钓鱼：pack 预设的 `baseURL` 和 `keyURL` 只接受 https，只有回环地址（`127.0.0.1`、`::1`、`localhost`）可以用 http。内置服务商的名称加入保留名单（见 6.4）。服务商下拉框里，插件预设和内置预设分组显示，插件预设一行写明 host 和来源插件。点「获取 API Key」时先显示目标域名，确认后才打开 keyURL。

**E5 翻译风格（P2）**
- 接入位置：设置 › 翻译 的提示词编辑区（`settings/translate.go:74-104`）加一组可选预设。
- 宿主照旧在后面追加 `subtitleReplyRule`（`packages/media/translate.go:34`），插件改不坏解析。

**E6 学习任务（P2）**
- 先把五处重复的异步 Chat 流程合并成一个 `runTask`，合并前先补测试。五处是：`state_explain.go` 的 `explainAsk`、`state_vocab.go` 里 `LookUpWord` 起的 Chat goroutine、`state_recap.go` 的 recap 请求、`palette_app.go` 的面板提问、`state_correct_candidates.go` 的 `correctCandidates`。
- task 数据包声明 `{scope: line|word|film-so-far, prompt, reply schema, render: sections|list|cards}`。宿主追加 JSON 回复规则，按播放进度截断上下文（不剧透），并记入每日 token 预算（依赖 P2.0 的 `ChatWith` 返回 usage）。
- 入口有两个：命令面板，以及播放器侧栏的「扩展」标签（见 E15）。

**E7 导出模板（P2）**
- 接入位置：在 `apps/kando/saved/export.go:40` 旁边新增学习导出注册表（`saved/targets.go`）。
- 数据包只能用宿主列出的占位符，提供 CSV/TSV 列映射和 Anki 笔记字段模板。

**E8 站点包（P4，最敏感）**
- 站点包不往 `kcore.VideoSites` 或 `sites/catalog` 里加任何东西。这两处都是进程级的全局状态（`sites/catalog/catalog.go:35` 的 `sync.OnceValue`，`packages/core/videosites.go:70` 的 `compiledVideoSites`），控制 socket 的浏览器扩展路径也复用同一套判断（`state_browserext.go:194,196`：注释里写着「Any program of the user can call the socket」，下面是 `if !browse.VideoLink(link) || !catalog.Load().On(link)`）。只要往里加，CLI、socket、MCP 的入口就会一起放宽。
- 改为单独做一个 `pluginSites` 匹配器，只给内置浏览器的两个入口用：`browsePageAsk` 和 `browsePageAdd`（`state_browse_buttons.go:52,102`）。`browserExtVideo` 和 remote 的处理函数明确不认插件站点，写测试锁住。
- 站点包加的下载一律不带 `--cookies-from-browser`。现在 `packages/media/tools.go:367-368` 只要开了浏览器登录，所有下载都会带上它，不分站点。所以队列给这类任务构造 Tools 时要把 `Cookies` 置空，具体接点在 P4 立项时核对。
- 前置条件（P4.0）：抽出共用的 `core.ValidateVideoSite`，只允许 Go 和 JS 共有的正则子集，不允许嵌套量词、反向引用和环视，长度不超过 256，域名唯一。站点包和 `pluginSites` 用它校验。`catalog.Load` 和 `compiledVideoSites` 不再需要改。
- 每个域名都要用户单独同意。不接受 sign-in cookie 配方，不新增 `core.Platform`。

### 2.2 脚本插件扩展点（P3 起，TSX 沙箱）

**E9 命令**
- 接入位置：`apps/kando/keys.go:83` 的 `slices.Concat(...)`，在最后加一项 `a.pluginKeys(cx)`。这一行就同时进入快捷键、macOS 菜单栏、命令面板、速查表和设置 › 快捷键。插件是异步载入的，载入后经 `update` → `Changed` → `areasFollow` → `menuFollow` 补进菜单。
- Group 用 `plugin:<id>`。`groupTitle`（`keys.go:57-79`）加一个分支，查清单里的名称。
- 插件按键和内置按键冲突时，清空插件的按键，降级为只在菜单和面板里出现，并在插件详情页显示冲突。
- 插件分组不能用 `groupPlayback`、`groupSubtitles`，否则会被 `keys_preset.go` 重映射。

**E10 在线查词来源**
- 和 E1 走同一条 `vocabResolve` 链，插件导出 `lookUp`，排在数据包词典之后、AI 之前。

**E11 字幕导出格式**
- `packages/core/export.go:136` 的 `Write` 和 `:139` 的封闭 switch 不动。宿主先用 `:53` 的 `Export.Cues` 算出 `[]core.ExportCue`，再调插件的 `exportSubtitles(formatID, cues)` 拿回字符串。写文件的是宿主，保存路径由宿主的文件面板决定。

**E12 学习导出目标**
- 插件导出 `exportNotes`，例如推到 AnkiConnect `127.0.0.1:8765` 或 Notion，接入 E7 的注册表。

**E13 标注器**
- 插件导出 `annotate(lang, lines)`，产出 E2 的 `Mark`。和 E2 一样由 `playing` 钩子按「视频 + 字幕哈希」按需批量计算，每批不超过 50 行。

### 2.3 后期扩展点（P5，各自单独立项）

**E14 候选译文和机器翻译来源（DeepL 等）**
- 先接 `packages/media/candidates.go:36`，作为改错的候选来源。之后再考虑在 `media.Tools` 上加可选字段 `Engine media.SegmentTranslator`：设了它，`Tools.Translate` 就委托过去，否则走 `ChatTranslator`。这样不改动任何一处 `media.Tools` 断言。
- 插件带凭据访问网络必须走 `kando.net.credentialFetch`，key 不进 JS（见第 6 节）。
- 插件导致的失败要能识别出来，所以先引入 `*media.StageError{Stage, Err}`，替代 `state_tools.go:238` 对错误前缀字符串的解析（`strings.Cut(job.Err.Error(), ": ")`）。

**E15 插件 UI**
- 播放器侧栏只加一个统一的「扩展」标签。`film/panel.go:26` 的 `var panelTabs` 改成 `film.App` 上的方法。标签内用二级选择器切换各插件，正文是 `tsx.View` 外包 `core.Catch`。
- 插件设置页以声明式 schema 为主（见 7.3），`tsx.View` 只作退路。

**E16 TTS、Transcriber**
- 不在本设计里承诺。

## 3. 插件形态与运行时

### 3.1 三层

| 层 | 形态 | 执行 | 平台 | 签名 |
|---|---|---|---|---|
| 0 外部自动化 | 现有的 CLI `--json`、控制 socket、`kando mcp`、`kando://` | 用户自己运行的程序，与用户同权 | 桌面 | 不适用，不算插件 |
| 1 数据包 | `.kandopack`：zip 或文件夹，包含 `kando-plugin.json` 和数据文件 | 不执行代码，宿主按 kind 解析 | 桌面；手机只拿到插件产出的数据 | 可选，未签名的只能从本地安装 |
| 2 脚本插件 | `.kandoplugin`：zip，包含 `kando-plugin.json` 和一个 NSGo 签名的 `.nsgopkg` | `nsgo/tsx`（goja 沙箱） | 仅桌面 | 必须签名 |

kind 是封闭枚举：`dictionary`、`wordlist`、`glossary`、`ai-service`、`translate-style`、`task`、`export-template`、`site`、`script`。遇到不认识的 kind，在设置页的问题区列出来，不加载。本轮（P0、P1）只实现 `glossary`、`ai-service`、`dictionary`、`wordlist` 四种；其余五种解析器认得，但 `Validate` 用哨兵错误 `ErrUnsupportedKind` 拒收，问题区显示「这个版本的看懂还不支持这类插件」，到各自那一期再放开（12.2 第 3 条）。

### 3.2 为什么选 nsgo/tsx

- 这是 NSGo 为「app 下载的代码」做的官方机制。`tsx/pkg/doc.go` 的原话是 "for code an app downloads: its own screens updated, or plugins"。
- 要用的东西都有了：`tsx.Load`、`Plugin.Call`、`Plugin.Exports`（`tsx/view.go`）；`Sandbox{Limits, Capabilities, Grant, Host, Storage}`；Harden（禁 `eval`、`Function`）；节点、定时器、请求和存储配额；按插件分区的 Storage 和 Secrets；`core.Host` 白名单 handler；Ed25519 根密钥 → 发布密钥的证书链、吊销列表和防回滚（`tsx/pkg/trust.go`）。它没有的东西写在 6.7 节。
- goja 是纯 Go 解释器：macOS hardened runtime 不需要 `allow-jit`，Windows 和 Linux 表现一致，交叉编译不受影响。
- 不新增 module。goja `v0.0.0-20261027234916-5f913d1693d9` 已经以 indirect 形式出现在 `apps/kando/go.mod:46`，只是会被链接进二进制。

### 3.3 已核实的运行时语义

- **线程**：`tsx.Load(cx *core.Scope, ...)`（`tsx/view.go:65`）需要一个活着的 Scope，并且在 `Limits.Load` 之内同步执行入口模块。`Plugin.Call`（`view.go:118`）的注释写着 "Call it on the engine's goroutine"，签名是 `Call(name string, args []any, done func(result any, err error))`。`pkg.Trust.Verify` 在引擎 goroutine 之外做。所以 `ready` 先在后台 goroutine 里做 Scan 和 Verify，再按每个 `a.update` 只载一个插件的节奏逐个 Load，每次 Load 的预算是 1s。这样 UI 最长只卡 1s，不会是 12 个插件 × 3s。
- **预算只算同步片段**：`tsx/sandbox.go:311` 的 `within` 只在最外层同步 JS 片段上挂 `Limits.Call` 计时器：`r.depth > 0` 时不计时，`fn()` 返回就停表，而且实现方式是 `time.AfterFunc(budget, func() { r.vm.Interrupt(ErrBudget) })`（`:322`）。所以 `await host.call(...)` 或 `await fetch` 的等待时间不算进预算。Kando 把每个同步片段的预算设为 500ms，Load 设为 1s：网络型插件不会被误杀，CPU 重的插件必须自己用 async 分片，否则会被打断。默认的 2s，以及 user-value 方案的 5s，都会让 UI 整段卡住，不采用。
- **Limits 的零值是「不限」**：`sandbox.go:24` 写的是 "Zero fields leave that unlimited"。`Harden` 是 bool，零值为 false。`:152-155` 是整体替换：`limits := DefaultLimits; if s.Limits != nil { limits = *s.Limits }`。所以 Kando 构造的每一份 Limits 都必须以 `tsx.DefaultLimits` 为底再覆盖字段，绝不写字面量：

  ```go
  // script.Limits: what every loaded plugin runs under.
  func Limits() *tsx.Limits {
  	l := tsx.DefaultLimits // Harden, Stack 1000, Nodes, Timers, Requests, Body, Storage stay set
  	l.Load = time.Second
  	l.Call = 500 * time.Millisecond
  	l.Timers = 64
  	l.Requests = 4
  	l.Body = 4 << 20
  	l.Storage = 1 << 20
  	return &l
  }
  ```

  要禁止节点就设 `Nodes = 1`，不能写 0，因为 `sandbox.go:299` 只在 `r.limits.Nodes > 0` 时才计数。不变量测试断言：Kando 构造的每份 Limits 都满足 `Harden == true`，且 `Stack`、`Nodes`、`Timers`、`Requests`、`Body`、`Storage` 都大于 0。
- **授权的语义**：`tsx.Load`、`tsx.View` 会经 `Sandbox.of`（`view.go:33-41`）把 nil 的 `Capabilities` 补成包自己签名的能力。所以对 Load 路径来说，nil Grant 的意思是「包声明的全部能力都生效，绕过用户的勾选」，并不是「宿主的全部权限」。真正等于宿主权限的只有一个入口：`tsx.Sandboxed` 在 `Capabilities` 和 `Grant` 都为 nil 时。这时连 `file://` 都能读本机文件，因为 `web.go:606` 只在 `caps` 为真时才拦 `file:`，`:655-656` 会直接 `os.ReadFile`。所以有三条规则：Kando 的每个 Load 都显式传入非 nil 的 Grant；全仓库禁止出现 `tsx.Sandboxed`，开发者模式也不例外；这两条都由测试强制（见第 9 节）。
- **Grant 从零构造**：Grant 只能写成 `&pkg.Capabilities{Network: ..., Host: ...}`，不能照抄包的 Capabilities 再删字段。原因是包本身可以声明 `secrets`、`webview`、`openUrl`、`clipboard`、`notifications` 等能力，而原生的 `openUrl`（`services.go:39-46`）只受 Capabilities.OpenURL 约束，不经过宿主确认。另外，Validate 直接拒收签名 Capabilities 里除 `Host`、`Network` 之外还有任何非零字段的包。
- **Host 才是真正的权限边界**：`core/host.go:124` 写明 handler 的 fn 是在调用它的那个 scope 的 Access 下跑的，插件的 Access 管不到 app 的 scope。所以 handler 只要用 `a.cx` 调 NSGo 服务，就带着 app 的全部权限。由此定三条不变量：每个插件有自己的 `*core.Host`，从不共享；handler 每次进来先查 `Effective`；handler 不在 `a.cx` 上替插件调用 OpenURL、Secrets、Storage，除非这是宿主自己的设计步骤（例如 `credentialFetch` 读宿主分区的凭据），并且已经过 `Effective` 检查。
- **host.call 超时**：`core/host.go:60` 的 `HostCallTimeout = 30 * time.Second`，JS 不传 `{timeout}` 就用它。Kando 的 Chat 客户端默认两分钟并且会重试，慢的 LLM 回复会在插件这边先超时。所以有三条规定：文档要求插件调 `kando.llm.chat` 和 `kando.net.credentialFetch` 时传 `timeout`（建议 150s）；宿主侧 handler 自己在 120s 时取消 context；`llm.chat` 上的 `ErrHostTimeout` 不计入熔断。

### 3.4 否决的方案

- **Go `plugin`**：不支持 Windows，需要 cgo，还要求宿主和插件用完全相同的工具链和依赖版本编译。Kando 要交叉编译 windows_x64/arm64，每次发版都会让所有插件失效。
- **原生 dylib**：会被 hardened runtime 的库验证拦下，而且等同于完全信任。
- **wazero 或其他 WASM**：要新增依赖，还得像 msime 那样自己设计 ABI、能力和签名，也没有 UI 能力，而 tsx 这些都有了。
- **第三方原生 sidecar 或 MCP 子进程（user-value 方案的第 4 层）**：插件会拿到用户的全部权限，能直接读 settings.json 里的明文 key。SHA-256 pin 防不住供应链攻击，Windows 上也没有分发先例，等于把 tsx 沙箱已经关上的口子重新打开。而且 `media.Command` 目前没有进程组、WaitDelay，也不能整树 kill。否决，不写进路线图。
- **WebView 作为插件 UI**：不走 tsx/pkg 的能力模型，焦点和按键也不经过 NSGo，每个面板还要开一个原生 web 视图。
- **控制 socket 作为插件协议**：它没有调用者身份，同用户的任何进程都能调用全部方法。继续只作为外部自动化层。
- **YAML 清单**：minimal-fit 方案用的是 YAML。Kando 的 settings.json、library.json 和同步协议都是 JSON，所以改用标准库 `encoding/json` 加 `DisallowUnknownFields`。格式统一，CLI 和以后 Go 写的服务端也能共用同一个校验器。

## 4. 清单格式

文件名统一为 `kando-plugin.json`，用 `encoding/json` 的 `Decoder.DisallowUnknownFields()` 严格解码，任何层级出现未知键就整包拒绝。

解码分两步：先只读 `schema`、`id`、`kind` 三个头部字段；kind 不在枚举里时返回 `ErrUnknownKind`；kind 在枚举里但本版还没实现时，返回只带头部的 Manifest，由 `Validate` 以 `ErrUnsupportedKind` 拒收，不对其余字段做严格解码（那些字段的类型要到对应的一期才定义）；kind 已实现时，再用 `DisallowUnknownFields` 严格解码全文。

通用规则：

- **id 的格式是 `<publisher>.<name>`**，必须匹配 `^[a-z0-9]+(?:[.-][a-z0-9]+)*$`，至少含一个 `.`，不超过 64 字节。这个正则排除了 `.`、`..`、`a..b`、以点或连字符开头和结尾的写法。第一段是发布者：`kando.` 保留给官方包；签名脚本包的发布者段必须和证书覆盖的 app-ID 模式一致（`ai.yldm.kando.plugin.<publisher>.*`），由 `plugins.Validate` 检查。未签名的数据包也用这个格式，但发布者段未经验证，来源徽标一律是「本地」，而且不能用 `kando.`。
- **路径安全**：所有按 id 拼出来的路径（`root/<kind>/<id>`、staging、Remove）在拼完之后都要过两道检查：`filepath.IsLocal(id)`，以及 `filepath.Rel(root/<kind>, 结果)` 不以 `..` 开头，确认结果确实位于 `root/<kind>/` 下面。
- `name`、`description` 以及所有 label 都是多语言 map，`en` 必填。
- `schema` 只在旧版 Kando 必须拒收时才升：数据包为 1，script 为 2，这样不支持脚本的旧版和手机会在版本检查时就拒绝。新功能通过新增键表达，旧版遇到不认识的键会整包拒收，自然安全地拒绝。
- **数据包的包形状**：平铺目录，最多 16 个文件，manifest 不超过 256 KiB，zip 文件不超过 32 MiB（dictionary 放宽到 200 MiB、64 个条目）。解压后的总字节数也设上限：数据包 64 MiB，dictionary 1 GiB。计数按实际读出的字节累加，不信任 `UncompressedSize64`；dictionary 里嵌套的 yomitan-zip 共用同一个计数器和条目上限，嵌套只允许一层。不允许符号链接，文件名只能是 ASCII，拦截 zip-slip 和绝对路径。每个文件都必须在 manifest 里被引用，`.md`、`.txt` 说明文件除外。
- **从文件夹安装**：用 `Lstat` 逐项检查，遇到符号链接或特殊文件就拒绝，再把普通文件复制进 staging，最后校验的是 staging 里的副本。不能先校验原目录再复制，那样有 TOCTOU 问题。
- **内容哈希**：对排序后的「路径、长度、字节」序列做 SHA-256，授权绑定在这个哈希上。

### 4.1 数据包示例

```json
{
  "schema": 1,
  "id": "example.jmdict-en",
  "kind": "dictionary",
  "version": "2026.11.0",
  "license": "CC-BY-SA-4.0",
  "author": "EDRDG (packaged by Example)",
  "name": {"en": "JMdict (English)", "zh-Hans": "JMdict（英文释义）", "ja": "JMdict（英語）"},
  "description": {"en": "Japanese–English dictionary for offline lookup."},
  "dictionary": {
    "format": "yomitan-zip",
    "files": ["jmdict_english.zip"],
    "from": ["ja"],
    "to": ["en"],
    "attribution": {"en": "JMdict © Electronic Dictionary Research and Development Group"}
  }
}
```

```json
{
  "schema": 1,
  "id": "example.jlpt",
  "kind": "wordlist",
  "version": "1.0.0",
  "license": "CC-BY-4.0",
  "name": {"en": "JLPT levels", "zh-Hans": "JLPT 等级"},
  "wordlist": {
    "language": "ja",
    "file": "jlpt.tsv",
    "tiers": [
      {"id": "n5", "label": {"en": "N5"}, "tone": "success"},
      {"id": "n3", "label": {"en": "N3"}, "tone": "accent"},
      {"id": "n1", "label": {"en": "N1"}, "tone": "warn"}
    ]
  }
}
```

```json
{
  "schema": 1,
  "id": "example.anime-ja-zh",
  "kind": "glossary",
  "version": "1.2.0",
  "license": "CC-BY-4.0",
  "name": {"en": "Anime terms (Japanese → Chinese)", "zh-Hans": "动画常用词（日→中）"},
  "glossary": {
    "from": "ja",
    "to": "zh-Hans",
    "terms": [
      {"source": "先輩", "target": "前辈"},
      {"source": "お兄ちゃん", "target": "哥哥"}
    ]
  }
}
```

```json
{
  "schema": 1,
  "id": "acme.corp-gateway",
  "kind": "ai-service",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Company LLM gateway"},
  "service": {"baseURL": "https://llm.example.com/v1", "model": "qwen3-32b", "keyURL": "https://llm.example.com/keys", "group": "cloud"}
}
```

数据包里不能出现 key、cookie、yt-dlp 参数或任何可执行内容。ai-service 的 `baseURL` 和 `keyURL` 必须是 https，只有回环地址可以用 http。

### 4.2 脚本插件示例

`capabilities` 用 `pkg.Capabilities` 的 JSON 形状，只允许出现 `host` 和 `network` 两个键，`network` 只允许 `http` 规则。每条规则都必须写 `methods`，并写 `paths` 或 `pathPrefixes`；`network.hosts`（放行任意请求）一律拒收。

```json
{
  "schema": 2,
  "id": "kando.ankiconnect",
  "kind": "script",
  "version": "1.2.0",
  "kando": ">=0.12.0",
  "license": "MIT",
  "author": "Kando",
  "homepage": "https://example.org/ankiconnect",
  "name": {"en": "AnkiConnect", "zh-Hans": "推送到 Anki", "zh-Hant": "推送到 Anki", "ja": "Anki に送る", "ko": "Anki로 보내기"},
  "description": {"en": "Send saved lines and words straight into Anki."},
  "script": {"package": "ankiconnect.nsgopkg", "sha256": "9f2c…"},
  "capabilities": {
    "host": ["kando.saved.read", "kando.words.read", "kando.sound.read"],
    "network": {"http": [{"host": "127.0.0.1", "port": 8765, "schemes": ["http"], "methods": ["POST"], "paths": ["/"]}]}
  },
  "optional": ["kando.sound.read"],
  "credentials": [],
  "contributes": {
    "exporters": [{"id": "notes", "takes": ["lines", "words"], "export": "exportNotes", "label": {"en": "Anki (AnkiConnect)", "zh-Hans": "Anki（AnkiConnect）"}}],
    "commands": [{"id": "sync", "export": "run", "key": "", "when": "always", "title": {"en": "Send new saved lines to Anki", "zh-Hans": "把新收藏的句子发到 Anki"}}]
  },
  "settings": [
    {"key": "deck", "type": "text", "default": "Kando", "max": 100, "title": {"en": "Deck", "zh-Hans": "牌组"}},
    {"key": "audio", "type": "bool", "default": true, "title": {"en": "Attach the line's sound", "zh-Hans": "附上句子的声音"}}
  ],
  "events": ["film.opened", "job.completed"]
}
```

AnkiConnect 的所有动作（包括删除牌组）都走同一个端点，动作名写在请求体里，`methods`/`paths` 管不到它。所以这个参考插件刻意不申请 `lines.seen` 和 `llm.chat`：它的行为由签过名的代码决定，不会被字幕内容左右。这条残余风险写在审核规范里（见 6.2）。

带凭据的插件（DeepL 候选来源）只多出下面这段。`methods` 和 `pathPrefixes` 是必填项，宿主据此限制 `credentialFetch`：

```json
"capabilities": {"host": ["kando.lines.seen", "kando.candidates.offer", "kando.net.credentialFetch"]},
"credentials": [{"id": "deepl", "title": {"en": "DeepL API key", "zh-Hans": "DeepL API 密钥"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["api-free.deepl.com", "api.deepl.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate"]}]
```

### 4.3 让贡献声明也受签名保护

`pkg.Contents`（`tsx/pkg/package.go:28`）只签 `App`、`Source`、`MinRuntime` 和 `Capabilities`，`kando-plugin.json` 在签名之外。所以 TSX 包必须导出 `kandoManifest()`，返回的对象要和清单的 `capabilities`、`contributes`、`credentials`、`settings`、`events` 完全一致。

宿主验签之后，在一个零授权的预读沙箱里调用它。注意：插件的模块顶层代码在用户同意之前就会在这里执行，这正是沙箱必须收紧的原因：

```go
l := tsx.DefaultLimits // keeps Harden, Stack, Body
l.Load, l.Call = time.Second, 250*time.Millisecond
l.Nodes, l.Timers, l.Requests, l.Storage = 1, 1, 1, 4<<10
probe := "pluginprobe." + randomHex(8) // throwaway partition, deleted right after
s := tsx.Sandbox{Grant: &pkg.Capabilities{}, Host: nil, Limits: &l, Storage: probe}
```

`Call("kandoManifest")` 一返回就 `Close()`，然后删掉 `probe` 分区。如果不设 `Storage`，`Sandbox.of` 会用 `App.ID` 作分区名（`view.go:37-39`），插件就能在安装前往一个卸载时不会被清理的分区里写数据。

然后逐字段比对，任何不一致都拒装，提示「这个文件被改动过」。此外还要满足：

- 包的 `Capabilities.Host` 必须等于清单 `capabilities.host` 加上 `optional`，包的 `Capabilities.Network` 必须等于清单的 `capabilities.network`，其他能力字段必须全部为零值。
- 包的 `App.ID` 必须是 `ai.yldm.kando.plugin.<id>`，并且被签名证书的 `Apps` 模式覆盖（`tsx/pkg/certificate.go:121-125`）。
- 清单声明的每个导出函数都必须出现在 `Plugin.Exports()` 里；用到的 host handler 必须都已声明，声明的也必须都用到（双向检查）。

这个缺口已提交为 [nsgo-dev/nsgo#761](https://github.com/nsgo-dev/nsgo/issues/761)：请求 `pkg.Build`/`pkg.Contents` 支持 app 自定义的签名文件条目（`doc.go` 的 Format evolution 一节说格式本身能容纳）。P3 的绕行代码处写 `// TODO(nsgo#761): pkg.Contents has no app-defined signed entries; compare kandoManifest() instead https://github.com/nsgo-dev/nsgo/issues/761`。

## 5. 宿主协议与 API

### 5.1 非界面包的 Go 类型草图（`apps/kando/plugins`）

类型按期加入，不给还没做的阶段预留字段或类型（12.2 第 3、4 条）。P0、P1 的部分：

```go
package plugins

type Kind string

const (
	KindDictionary Kind = "dictionary"; KindWordlist Kind = "wordlist"; KindGlossary Kind = "glossary"
	KindAIService Kind = "ai-service"; KindStyle Kind = "translate-style"; KindTask Kind = "task"
	KindExportTemplate Kind = "export-template"; KindSite Kind = "site"; KindScript Kind = "script"
)

// Text is plugin-supplied text by language tag; "en" is required.
type Text map[string]string

// In picks lang (as i18n.Language returns it), walking i18n.Parent, then "en". Call it only inside bindings so a locale switch follows.
func (t Text) In(lang string) string

// Manifest has a field per kind implemented so far; a known kind not implemented yet is read only as far as its header (Schema, ID, Kind) and Validate refuses it with ErrUnsupportedKind.
type Manifest struct {
	Schema      int         `json:"schema"`
	ID          string      `json:"id"`
	Kind        Kind        `json:"kind"`
	Version     string      `json:"version"`
	License     string      `json:"license"`
	Author      string      `json:"author,omitempty"`
	Homepage    string      `json:"homepage,omitempty"`
	Name        Text        `json:"name"`
	Description Text        `json:"description,omitempty"`
	Dictionary  *Dictionary `json:"dictionary,omitempty"` // P1.3
	Wordlist    *Wordlist   `json:"wordlist,omitempty"`   // P1.4
	Glossary    *Glossary   `json:"glossary,omitempty"`   // P1.1
	Service     *Service    `json:"service,omitempty"`    // P1.1
}

type Pack struct {
	Manifest
	Dir, Hash string
}

type Issue struct{ Dir string; Err error } // English sentinel errors; the app maps them to catalog text

var ErrUnknownKind, ErrUnsupportedKind error // and the other sentinels of 7.5

func Parse(data []byte) (Manifest, error)
func Validate(src string) (Pack, error)      // same code for app scan, `kando plugin validate` and a future Go server
func Scan(root string) ([]Pack, []Issue)
func Stage(root, src string) (staged string, p Pack, err error) // copy (Lstat, byte-counted) into root/.staging-<rand>, Validate the copy
func Swap(root, staged string, p Pack) error // old → .old-<rand>, staged → root/<kind>/<id>; caller closes open handles first
func Remove(root, id string) error           // id checked with IsLocal + Rel before any path is built

// State is plugins/state.json: single writer, tmp+rename, never synced, never in settings.json.
type State struct {
	Format  int           `json:"format"`
	Plugins []PluginState `json:"plugins"`
}
type PluginState struct {
	ID      string `json:"id"`
	Hash    string `json:"hash"`
	Enabled bool   `json:"enabled"`
}
```

P1 的数据包没有授权：glossary、ai-service、dictionary、wordlist 装好就启用（6.2 节），`Hash` 只用来认出「同一个 id 换了内容」，好在列表里标出「已更新」。

P3 起追加的部分（脚本插件；本轮不写）：

```go
// added to Manifest in P3
	Kando        string            `json:"kando,omitempty"`
	Script       *Script           `json:"script,omitempty"`
	Capabilities *pkg.Capabilities `json:"capabilities,omitempty"` // only Host and Network (HTTP rules) may be set
	Optional     []Perm            `json:"optional,omitempty"`
	Credentials  []Credential      `json:"credentials,omitempty"`
	Contributes  *Contributes      `json:"contributes,omitempty"`
	Settings     []Setting         `json:"settings,omitempty"`
	Events       []string          `json:"events,omitempty"`

// P2 adds Style, Task and Template the same way; P4 adds Site.

type Credential struct {
	ID           string   `json:"id"`
	Title        Text     `json:"title"`
	Header       string   `json:"header"`
	Format       string   `json:"format"`
	Hosts        []string `json:"hosts"`
	Methods      []string `json:"methods"`
	PathPrefixes []string `json:"pathPrefixes"`
}

// added to Pack in P3
	Source Source // official | verified | developer | local; for scripts derived from the certificate chain, never from the manifest
	Signer string // release key ID from the certificate; "" for unsigned data packs

// added to PluginState in P3, with State.Format raised
	OldHash string `json:"oldHash,omitempty"` // tells "updated, re-grant" apart from "never granted"
	Signer  string `json:"signer,omitempty"`  // release key ID the grant was given to
	Grant   Grant  `json:"grant"`
	Budget  int    `json:"budget,omitempty"`  // daily LLM tokens, 0 = default

// Grant is what the user agreed to, in full: an update keeps it only if what it now declares is a subset of each part.
type Grant struct {
	Perms       []Perm         `json:"perms,omitempty"`
	Network     []pkg.HTTPRule `json:"network,omitempty"`
	Credentials []Credential   `json:"credentials,omitempty"` // id, hosts, header, format, methods, paths as granted
	Sites       []string       `json:"sites,omitempty"`       // P4
}

// Effective is grant ∩ declared ∩ the package's signed Capabilities ∩ current gates (offline, locked library).
func (s State) Effective(p Pack, offline, locked bool) Grant

// Health is plugins/health.json. Reason ∈ failed|slow|init|revoked|hung.
type Health struct{ ID, Hash, Reason string }

// InFlight marks (id, hash) as running a call; a mark left at the next start means that call hung or crashed the app.
func MarkInFlight(root, id, hash string) error
func ClearInFlight(root, id string) error
```

数据包的查询接口，各扩展点只认这几个窄接口：

```go
// Dict is an open dictionary pack; Manifest.Dictionary is what its manifest declares.
type Dict interface {
	ID() string
	LookUp(ctx context.Context, word, base, language string) (lookup.Entry, bool, error)
	Close() error // the index file stays open for binary search; Swap needs it closed
}

// in apps/kando/film/subtitle (leaf package), so film does not import plugins
type Mark struct {
	Start, End int    // byte offsets in the line
	Ruby       string
	Tone       string // accent|warn|success|muted, mapped by the host to theme roles
	Underline  bool
	Tip, Tier  string
}
```

替换同 id 包的顺序：先在引擎 goroutine 上关掉这个包的词典句柄并 Close 它的脚本，然后把旧目录重命名为 `.old-<rand>`，再把 staging 重命名为目标目录，最后重新打开。对已存在的非空目录直接 rename 在任何平台上都会失败，所以必须先挪走旧目录。Windows 上如果碰到共享冲突，先重试几次；还不行就记一个待替换标记，下次启动、还没打开任何句柄时再换。

### 5.2 app 一侧（package main）

```go
// state_plugins.go
type pluginsState struct {
	packs   []plugins.Pack
	issues  []plugins.Issue
	state   plugins.State
	pending *plugins.Pack                 // install confirmation (P1.1)
	dicts   []plugins.Dict                // open dictionaries in lookup order (P1.3)
	marks   map[string][][]subtitle.Mark  // video + subtitle hash -> per line, immutable once set (P1.4)
	// P3 adds:
	health  map[string]plugins.Health
	scripts map[string]*tsx.Plugin        // engine goroutine only
	hosts   map[string]*core.Host         // one per plugin, never shared
	loading []string                      // ids still to Load, one per update tick
}

// savedPlugins holds preferences only, never trust: dictionary order, annotation switches, chosen exporter.
type savedPlugins struct {
	PluginDictOrder []string         `json:"pluginDictOrder,omitempty"`
	PluginMarks     savedPluginMarks `json:"pluginMarks,omitempty"`
}
// clean() trims, dedupes, and keeps ids that are missing now (their disk may be unmounted).

func (a *app) pluginsHooks() areaHooks
func (a *app) pluginKeys(cx *core.Scope) []shortcut // P3
func (a *app) pluginsOverlay(cx *core.Scope) core.Node
func (a *app) pluginTerms(job core.Job) []core.Term
func (a *app) PluginPresets() []settings.Preset
func (a *app) llmPresets() []settings.Preset // settings.Presets(m) + PluginPresets(), for PresetByID/ProviderID
func (a *app) vocabResolve(ctx context.Context, q lookup.Query) (lookup.Entry, vocabSource, bool)
```

`pluginsHooks` 各回调：

- `restore`：读 savedPlugins。
- `save`：写 savedPlugins。
- `ready`（P1）：在后台 goroutine 里读 state.json、Scan，经 `a.update` 交回引擎，打开已启用的词典。
- `ready`（P3 起追加）：在后台 goroutine 里读 health.json。如果发现上次留下的 in-flight 标记，就把对应的 (id, hash) 记为 `hung` 并停用它。然后 Scan、Verify，把结果交回引擎，再按每个 `a.update` 只 Load 一个插件的节奏逐个载入。每载入一个都会触发 `Changed`，菜单和面板经 `areasFollow` → `menuFollow` 补上插件行，不靠 hook 的先后顺序。
- `playing`（P1.4）：当前影片的「视频 + 字幕哈希」和缓存的键不同时，起后台计算标注（P3 起同时推送 `film.opened` 事件）；其余时候只比较键、什么也不做，返回 false。
- `shelfChanged`（P3）：只在任务完成时推 `job.completed`，不计算标注。
- `llmChanged`（P3）：推 `llm.changed`。
- `afterChange`：返回 false。
- `quitStarting`：关掉词典句柄（P1.3）；P3 起还要 Close 所有插件。这时引擎还在跑，`a.cx` 还在。不用 `quitting`，因为那时引擎已经停了，app 的 scope 也没了，而 `Plugin.Close` 要释放的正是那个 scope 的子 scope。

在 `events.go` 的 `areas()` 里，它放在 `settingsHooks` 之后。

后台调用插件的桥：

```go
// pluginCall runs a plugin export from any goroutine and waits with ctx.
func (a *app) pluginCall(ctx context.Context, id, name string, args ...any) (json.RawMessage, error) {
	out := make(chan result, 1)
	a.update(func() {
		p := a.plugins.scripts[id]
		if p == nil { out <- result{err: plugins.ErrNotLoaded}; return }
		hash := a.pluginHash(id)
		plugins.MarkInFlight(a.pluginsRoot(), id, hash) // a hang or crash inside the call leaves it for the next start
		p.Call(name, args, func(v any, err error) {
			plugins.ClearInFlight(a.pluginsRoot(), id)
			out <- result{v: v, err: err}
		})
	})
	select {
	case r := <-out: a.pluginsCount(id, r.err); return r.json()
	case <-ctx.Done(): return nil, ctx.Err()
	}
}
```

in-flight 标记从 `Call` 开始一直保留到 `done` 回调，所以 await 之后恢复执行的那些同步片段也在覆盖范围内。每次调用都要写一次小文件，这个代价算进 P3.0 的测量。每次调用的数据量也有上限，例如一批不超过 50 行，失败就丢弃这次的结果。

### 5.3 插件能调用的宿主能力

插件唯一的入口是它自己的 `*core.Host`。handler 名就是权限名，检查有三层：包的 `Capabilities.Host`、`Sandbox.Grant`，以及 handler 闭包里再查一次 `Effective`。越权一律返回固定的错误码 `denied`，不暴露内部错误文本。

```go
type Perm string

const (
	PermFilmCurrent     Perm = "kando.film.current"        // {film, title, spoken, language, positionMs}
	PermLineCurrent     Perm = "kando.line.current"        // {index, original, translation}
	PermLinesSeen       Perm = "kando.lines.seen"          // only up to the played horizon (explain.Request/recap.Request rules)
	PermSavedRead       Perm = "kando.saved.read"          // saved lines, no file paths
	PermWordsRead       Perm = "kando.words.read"
	PermSoundRead       Perm = "kando.sound.read"          // base64, ≤2 MiB
	PermGlossarySuggest Perm = "kando.glossary.suggest"    // proposals only, OriginAuto, never over Locked
	PermCandidatesOffer Perm = "kando.candidates.offer"
	PermLLMChat         Perm = "kando.llm.chat"            // proxied, budgeted
	PermCredFetch       Perm = "kando.net.credentialFetch" // host-injected secret, only to the granted credential's hosts, methods and paths
	PermToast           Perm = "kando.toast"
	PermOpenURL         Perm = "kando.ui.openURL"          // https only, host confirm every call
)
```

`kando.settings.get` 不需要权限，只返回这个插件在 `settings` 里声明的那些值。

插件侧调用（TSX）：

```ts
import { host } from "nsgo"
const now = await host.call("kando.film.current", {})
// -> {"film":"f_3a9c…","title":"…","spoken":"ja","language":"zh-Hans","positionMs":82100}
const lines = await host.call("kando.lines.seen", { limit: 20 })
// -> [{"index":40,"startMs":80100,"endMs":82000,"original":"…","translation":"…"}, …]
const r = await host.call("kando.llm.chat", { system: "…", user: "…", maxTokens: 400 }, { timeout: 150000 })
// -> {"text":"…","model":"qwen3-32b","tokens":512}   never BaseURL or key
const d = await host.call("kando.net.credentialFetch", {
  credential: "deepl", method: "POST", url: "https://api-free.deepl.com/v2/translate",
  header: { "Content-Type": "application/json" }, body: JSON.stringify({ text: ["…"], target_lang: "ZH" })
}, { timeout: 150000 })
// -> {"status":200,"body":"…"}   secret injected by host, no redirects, only Content-Type kept

export async function lookUp(q) { return { word: q.word, reading: "…", meaning: "…" } } // or null
export async function exportSubtitles(formatID, cues) { return "…" }
export async function exportNotes(items, settings) { return { sent: 12, skipped: 0 } }
export async function run(commandID) { return { toast: "…" } }
export function kandoManifest() { return { /* identical to kando-plugin.json fields */ } }
```

宿主推给插件的事件（`Host.Send`）只带标量，正文要靠有权限的 getter 去取：

```json
{"name":"film.opened","data":{"film":"f_3a9c…","spoken":"ja"}}
{"name":"job.completed","data":{"film":"f_3a9c…","stage":"complete"}}
{"name":"llm.changed","data":{}}
```

`line.changed` 不推送，插件需要当前行时自己调 `kando.line.current`。影片 id 是按插件派生的 `HMAC(pluginSecret, filmKey)`，不同插件之间关联不起来，也不泄露路径。

`kando.llm.chat` 的处理流程：先查预算 → 过 `paletteTranslator` 的门控（离线、隐私、未配置）→ 在 goroutine 里调 `ChatWith` → 经 `a.update` 回来 Resolve。现有的 `ChatTranslator.Chat`（`packages/media/translate.go:230`）只返回字符串，不读 usage，不设 `max_tokens`，请求头也只有 Content-Type 和 Authorization（`:609-611`）。所以 P2.0 先在 `packages/media` 加一个 API：`ChatWith(ctx, ChatRequest{System, User, MaxTokens, Header}) (ChatReply{Text, Usage}, error)`，`Chat` 改成调它，行为不变。服务商不返回 usage 时，按字符数估算 token。请求头带 `X-Title: Kando plugin <id>`，用户在服务商的账单里能分辨出来。

### 5.4 外部接口

- CLI 在 `cli/cli.go:148-165` 的命令表里加一行 `plugin`，子命令有 `list`、`validate <file|dir>`、`install <file>`。
  - `install` 只把文件放进 `plugins/inbox/`，用户在 app 里确认后才会安装。
  - `validate` 离线运行，和 app 用同一个 `plugins.Validate`。
  - dev root 的生成和开发者包的签名不放在 CLI，也不经过 socket（见 6.4）。原因有两个：CLI 进程没有 NSGo Scope，拿不到 `core.Secrets`；socket 对同用户的任何程序都开放，不能让它替别人签包。
- 控制 socket 只加只读的 `plugins.list`，经过 `lockRemoteGate`。不加 install、enable、grant。
- `kando://`、浏览器扩展、MCP 都不能触发安装或授权。

## 6. 权限与安全模型

### 6.1 资产与威胁

要保护的资产：

1. 用户的 LLM key（目前以明文存在 settings.json，`settings_ai.go:22-38`）。
2. 插件代管的第三方凭据（DeepL、Notion 的 key）。
3. 从浏览器复制来的登录 cookie（`sites/catalog/signin.go`）。
4. 片库、学习记录和观看进度。
5. 用户的 token 额度，也就是钱。
6. 机器本身，以及窗口的可用性。
7. 看懂的信任闸门：`catalog.On`、`VideoLink`、socket 和 native host。

威胁：

- T1 恶意插件：窃取 key、cookie 或字幕，或者刷用户的 token。
- T2 供应链：发布密钥被盗、回滚到旧版本、同名抢注。
- T3 恶意数据包：用 ReDoS 正则拖垮页面；用站点包放宽信任闸门；参数注入；路径穿越和 zip bomb；用 ai-service 预设钓鱼。
- T4 UI 仿冒：骗用户输入 key；名称里用 Bidi 或零宽字符冒充官方或内置服务商。
- T5 资源滥用：卡死引擎 goroutine，或者耗尽内存。
- T6 间接提示注入：字幕内容进入插件的 LLM 请求，诱导插件越权。
- T7 同用户的本机恶意程序：不在防护范围内，因为它本来就能直接读 settings.json。但插件系统不能让它更容易得手，尤其不能让它借插件拿到 macOS 钥匙串里受 ACL 保护的凭据。

### 6.2 能力授权

- 安装不等于授权。
  - 数据包：glossary、dictionary、wordlist、ai-service、translate-style、export-template 安装后默认启用。`task` 要消耗 token，必须先授 `kando.llm.chat`。`site` 必须逐个域名同意。
  - 脚本：安装后处于停用状态，详情页的能力一项都不勾，用户勾选并打开开关才算授权。勾一项时，它依赖的能力一起勾上（例如 `llm.chat` 依赖 `lines.seen`）。
- 授权记录是完整的 `Grant`：权限名、network 规则的完整集合、每个凭据的 (id, hosts, header, format, methods, pathPrefixes)，以及站点域名。不再只存权限名。
- 实际生效的能力 = 包签名里的 `Capabilities` ∩ 清单声明 ∩ 用户授予的 `Grant` ∩ 当前状态。
  - 离线时 `Grant.Network` 为 nil，`llm.chat` 和 `credentialFetch` 拒绝。
  - 库被锁住时（与 `lockRemoteGate` 同义），所有读片库的 handler 拒绝。
- 授权写在 `<UserConfigDir>/Kando/plugins/state.json`，绑定内容哈希和签名者。它不进 settings.json，因为那个文件会整份重写、会丢掉不认识的键，而且离明文 key 太近；也不同步到手机或账号。
  - 包变了一个字节：如果新版本由同一个签名者签名，并且新声明的每一部分（权限、network 规则、每个凭据的 hosts/header/format/methods/paths、站点）都是旧授权的子集，授权沿用。任何一项超出旧授权，就视为扩大能力：插件按「旧授权 ∩ 新声明」继续运行，凭据只要有变化就整体暂停注入，并标出「已更新，需要重新授权」。
  - 签名者变了：授权、Storage 和凭据分区全部作废，按新安装处理。
- 每个插件的配额：每分钟最多 60 次 `host.call`；`llm.chat` 默认每天 50k token，可以调整，用量在详情页可见；同时启用的脚本插件最多 12 个。
- **网络能力本身就是副作用通道**。有了 `lines.seen`（字幕内容可能来自攻击者）和 `llm.chat`，插件就能按 LLM 的输出发出有副作用的网络请求，不经过任何确认。所以 T6 防不住，只能缩小它的影响：
  - 安装页和详情页把「读你的字幕或用你的 AI」与「联网或 credentialFetch」同时勾选的组合单独标成高风险，单独一行说明。
  - network 规则必须写死 Methods 和 Paths/PathPrefixes（4.2 节，由 Validate 强制）。审核规范要求插件用最窄的规则；像 AnkiConnect 这种所有动作共用一个端点的服务，规则缩不到动作这一级，在审核规范和详情页如实写出来。
  - `credentialFetch` 只允许授权凭据里列出的 methods 和 pathPrefixes。
  - 真正只有宿主能拦住的副作用，才做成「每次确认」（`openURL`）或「只出建议」（`glossary.suggest`、`candidates.offer`，复用 `core.Proof` 的接受和回滚模型）。

### 6.3 密钥隔离

- 插件没有文件系统能力，`pkg.Capabilities` 里本来就没有这一项；Grant 从零构造，`tsx.Sandboxed` 全仓禁用，所以 `file://` 也读不到。插件读不到 settings.json、library.json 和 cookie 文件。
- Kando 自己的 LLM key 只通过 `kando.llm.chat` 代理使用，并沿用 `SameHost` 的 key 与 host 绑定。
- **分区**：
  - 插件 JS 可见的 `Sandbox.Storage` 分区名是 `plugin.<id>.<signerKeyID>`。
  - 宿主代管的凭据放在另一个分区 `pluginhost.<id>.<signerKeyID>`，由宿主在自己的 scope 上用 `cx.ProvideSecrets` 读写。JS 的 `nsgo.Secrets.get` 只能碰到 `Sandbox.Storage` 那个分区（`tsx/sandbox.go:164-167`），所以读不到凭据；何况 Grant 里也不给 `secrets`。
  - 分区名带上签名者，所以同 id、换签名者的包继承不到旧分区。签名者一变，宿主就清空旧分区，要求用户重新输入凭据。
- 凭据只能由宿主渲染的 secret 字段写入。输入框上直接写「此密钥只会发送到：<hosts>」，保存后界面只显示「已保存」，提供「替换」和「清除」。插件只能通过 `credentialFetch` 间接使用凭据：URL 的 host、method、path 都在授权范围内才注入，不跟随重定向，响应头只保留 `Content-Type`，`Set-Cookie` 丢弃。
- 宿主内置一张常见服务的 host 表（DeepL、Notion 等）。凭据的 id 或标题指向这些服务、hosts 却不在表里时，或者非官方插件把凭据发往表外的主机时，安装页和输入框额外显示警示。
- Linux 上没有 Secret Service 时（`ErrSecretsUnavailable`），声明了 `credentials` 的插件直接禁用并说明原因，不退回明文。
- 把 Kando 自己的 LLM key 从 settings.json 迁进 `core.Secrets` 是值得做的加固，但它不是插件系统的前提，列为独立 PR，由 owner 决定（见第 12 节）。

### 6.4 完整性与签名

- 脚本包只接受能链到受信根的 `.nsgopkg`（`pkg.Trust.Verify`，Trust 目录为 `<UserConfigDir>/Kando/plugins/trust`）。
- **根密钥每次启动都要重新加入**：`Trust.AddRoot`（`tsx/pkg/trust.go:66-72`）只在内存里追加，`OpenTrust` 只恢复吊销列表和已接受的 build。所以 app 每次启动都要先 AddRoot 内嵌的官方插件根公钥；开发者模式开着时，再加入存在 `core.Secrets` 里的 dev root 公钥。dev root 公钥不放普通文件，这样同用户的程序没法悄悄塞进一把自己的根。
- Kando 新建一把离线的插件根密钥，和应用更新用的 `NSGO_UPDATES_KEY`（`nsgo.yaml:31-33`）分开。用 `go tool nsgo certify` 给发布密钥签证书，限定 app-ID 前缀：官方是 `ai.yldm.kando.plugin.kando.*`，第三方是 `ai.yldm.kando.plugin.<publisher>.*`，和 id 的发布者段对齐（第 4 节）。证书有效期不超过一年。
- 吊销列表用 `go tool nsgo revoke` 生成，随 appcast 一起发布。`Trust.Accept` 持久化已接受的最高 build，用来防回滚。
- 联网时每天拉一次吊销列表。列表过期时，只暂停申请了网络或凭据的非官方插件（依据 `Package.ExpiredRevocations`），不在全局调用 `RequireFreshRevocations`，免得长期离线的用户连纯本地插件都用不了。被吊销的插件立刻 Close，并显示「已被撤回」。
- 可选的加固：把每个根已见过的最高吊销序号，以及每个插件已接受的最高 build，额外存一份摘要到 `core.Secrets`。启动时和 `plugins/trust/trust.json` 比对，发现回退就停用所有非官方脚本插件。是否做由 owner 决定（见 12.3 第 2 条）。
- 来源徽标（官方、已验证、开发者、本地数据包）由证书链推出，不读清单。插件名去掉 Bidi 控制符和零宽字符，最长 40 个字符。保留名单是「Kando」「看懂」「Official」「官方」，再加上所有内置 AI 服务商的名称（取自 `settings.Presets` 的显示名），比较时忽略大小写和全半角。
- **开发者模式**是设置里的显式开关。dev root 的生成和签名都在 app 里完成，位置是设置 › 插件 › 开发者：「生成开发者密钥」把私钥和公钥都存进 `core.Secrets`；「打包并安装文件夹…」由用户在系统文件面板里选一个源码文件夹，app 用 dev key 签名，然后走和正式包同一条 Stage → Verify → 比对 kandoManifest → 确认 sheet 的路径。不允许用 `tsx.Sandboxed` 直接跑源码，CLI 和 socket 都不能签。开发者包一直带着徽标，不参与更新，不能分享安装，也不允许占用已安装的非开发者包的 id。
- 数据包可以不签名。将来如果支持签名数据包，site 包的高风险能力只对官方和已验证来源开放。

### 6.5 安装与更新

- 安装入口只有三个：设置 › 插件页的「从文件安装…」、把文件拖进窗口（`desktop.DroppedPaths`）、CLI inbox。三者最后都要经过宿主画的确认 sheet。开发者包另有 6.4 节的入口，同样要经过确认 sheet。
- 安装流程：
  1. `plugins.Stage`：复制或解压到 `plugins/.staging-<rand>`。zip 按实际字节计数（嵌套 zip 共用计数器），拦截 zip-slip、符号链接和绝对路径；文件夹用 Lstat 逐项复制。
  2. 对 staging 里的副本跑 `plugins.Validate`，包括 id 正则和路径检查。脚本包还要 Verify，并在预读沙箱里比对 `kandoManifest()`。
  3. 静态检查：声明的 host 权限都在已知 handler 表里；签名 Capabilities 里除了 Host 和 Network 没有别的字段。
  4. 用户在确认 sheet 上点「安装」后，在引擎 goroutine 上关掉同 id 包的句柄，再 `plugins.Swap`（5.1 节的顺序）。
  5. 残留的 `.staging-`、`.old-` 目录超过 24 小时才清理。
- 每次启动 Scan 时，用同一个校验器再校验一遍。
- 第一版不做在线更新：更新就是安装一个新文件，走同样的确认流程，授权按 6.2 节的子集规则处理。将来做社区索引时，只从签名索引里列出的条目自动更新；能力扩大的版本必须由用户确认；离线模式下不检查。
- 插件文件永远不写进 `.app` 包，只放在 `<UserConfigDir>/Kando/plugins/`。

### 6.6 隔离、熔断与审计

- 每次调用都包在 recover 和超时里，失败就整段丢弃这次的产出。
- 熔断按 (id, hash) 计：init 失败立即停用；最近 50 次调用里有 3 次出错或超出 `ErrBudget`，也停用；`llm.chat` 上的 `ErrHostTimeout` 不计入。结果写进 `plugins/health.json`，只存类别。
- **卡死和崩溃后自愈**：每次 Call 之前先把 (id, hash) 写成 in-flight 记录，回调时清掉（5.2 节）。下次启动时如果发现残留的记录，就把那个插件标为 `hung` 并自动停用，详情页说明「上次运行时让看懂停止响应」。这一条覆盖了 Limits 拦不住的卡死和 OOM（见 6.7）。
- `plugins/audit.log` 记录安装、授权、吊销、熔断、确认框结果，以及每次 `llm.chat` 的 token 数，不记录内容。写盘前先经过 `feedback.Scrubber`。

### 6.7 残余风险（在授权页如实写出）

- **恶意或写坏的插件可以让窗口永久卡死，或者让整个 app 内存耗尽**，Limits 兜不住这两种情况。第一，时间预算靠 `vm.Interrupt` 实现，但 goja 的回溯正则走 regexp2（`goja/regexp.go:58`），没有设 MatchTimeout（`regexp2` 的 `DefaultMatchTimeout` 是 `math.MaxInt64`），执行原生匹配时也不检查中断标志。一句 `/(a+)+\1$/.test("a".repeat(40)+"!")` 就能让引擎 goroutine（也就是 UI 线程）一直卡住，500ms 的预算不会触发。这一点已用 goja 最小复现确认：200ms 时发出 Interrupt，10s 后匹配仍在运行，已提交为 [nsgo-dev/nsgo#759](https://github.com/nsgo-dev/nsgo/issues/759)。第二，`Limits` 没有任何堆内存上限，一个同步片段在 500ms 内就能分配出几个 GB，已提交为 [nsgo-dev/nsgo#760](https://github.com/nsgo-dev/nsgo/issues/760)。缓解手段只有 6.6 节的 in-flight 自愈（下次启动时自动停用）和签名问责。在上游修好之前，P3.0 spike 的放行门槛要把这一项算进去，P3 的绕行处写 `// TODO(nsgo#759): ...` 和 `// TODO(nsgo#760): ...`，带上 issue 链接。
- **本机状态文件对同用户可写**：同用户的进程可以改写 `state.json`（授权）、`health.json`（熔断），以及 `plugins/trust/trust.json`（已接受的最高 build 和吊销列表）。删掉或改掉 trust.json，防回滚和吊销就会清零：`OpenTrust` 在文件不存在时返回一个空的 Trust，而 "a root without a list revokes nothing"。所以「防回滚」和「已被撤回」只防得住网络上的攻击者，防不住本机程序。msime 尝试过 keychain HMAC，最后放弃了，这里不假装能防住。6.4 节的可选加固能把回退检测出来。
- **凭据库按平台不同**：macOS 钥匙串条目的 ACL 信任写入它的那个二进制，其他同用户进程读不到插件的凭据。本设计让分区带上签名者、dev root 公钥也存进钥匙串，就是为了不让同用户程序借「同 id 换包」或「塞一把自己的根」拿到这些凭据。Windows 凭据管理器和 Linux Secret Service 对同用户进程本来就开放，在这两个平台上，同用户的恶意程序能直接读出插件凭据，插件系统不额外放大也无法缩小这个风险。
- **网络是副作用通道**：获准联网的插件，可能被字幕内容诱导着，发出它声明范围内的任何请求（6.2 节）。
- goja 本身也可能有其他漏洞，不是零风险。

## 7. UX

### 7.1 入口与列表

- 设置里新增一节「插件」：`settings/sectionid` 加 `Plugins`；`settings/sections.go:32` 的 `var sections` 改成函数；页面写在 `settings/plugins.go`，用 `settings/form` 的 Card、Row、SwitchRow 和 `ui.Switch` 拼。
- 列表按 kind 分组。每行有图标、名称、来源徽标、版本·作者、行内开关和状态。状态包括：需要授权 / 已更新，需要重新授权 / 已停用：出错 / 已停用：太慢 / 已停用：上次让看懂停止响应 / 已被撤回 / 离线时不可用 / 未安装（savedPlugins 引用了，但目录里没有）。
- 词典分组可以拖动排序，决定查词时谁先答。显示顺序和执行顺序分开管理。
- 顶部按钮：「从文件安装…」（系统文件面板，配合 `holdModal()`），以及「在访达中显示插件文件夹」（`desktop.Reveal`）。开发者模式开着时，多一组「开发者」：生成开发者密钥、打包并安装文件夹…。
- 问题区列出没能载入的包，以及原因的本地化文案。

### 7.2 安装确认（`pluginsOverlay` 里的 `ui.Dialog`）

- 显示名称、来源（例如「由 acme 发布，看懂已验证」，取自证书）、版本、大小、哈希前 12 位。
- 「它想要」的清单：每个权限的说明来自 catalog，插件不能自定义权限说明。例如「读取正在播放的影片到当前句为止的字幕」「用你的 AI 服务回答，每天最多 5 万 token」「向 api.deepl.com 发送 POST /v2/translate」。必需权限在上，可选权限在下，默认不勾。
- 「读你的字幕或用你的 AI」与「联网」同时出现时，单独一行标为高风险。凭据一行写明它会发往哪些主机；主机不在内置服务表里时额外警示。
- 开发者来源加一条警示。site 包每个域名一行复选框。
- 点「安装」后，脚本插件仍处于停用状态，回到列表打开开关才算授权。

### 7.3 详情页

- 说明和出处（词典必须显示 attribution）。
- 权限按「读取你的内容 / 改动你的内容 / 联网 / 用你的 AI」分组，每一项都能单独撤回。
- 每日 token 用量和预算。
- 插件声明的设置，类型有 bool、choice、number、text、secret，最多 16 项，由宿主用 `settings/form` 加 `ui.Switch`、`ui.Select`、`ui.Input` 渲染。secret 字段的输入框上写着「此密钥只会发送到：<hosts>」。值存在 `plugins/<id>/settings.json`（secret 除外，它在 `pluginhost.` 分区里），不计入哈希，改设置不需要重新授权。
- 按键冲突提示、「查看审计记录」、「卸载」。卸载会删除目录、`plugin.<id>.*` 和 `pluginhost.<id>.*` 两个分区，并从 state.json 移除，删除前要确认。AI 预设插件留下的服务记录会保留，key 不丢。

### 7.4 角色选择的位置

- 设置 › 外观 › 逐词标注：注音或拼音开关、「按等级着色：JLPT」、「标出已学词」。
- 查词卡在底部标出来源，例如「JMdict」或「AI」。学日语或中文的用户第一次点词时，卡片底部提示「装离线词典，查词更快、不耗 AI」，点了打开设置 › 插件。
- 设置 › AI 的服务商下拉框里，插件预设单独成组，每项标出 host 和来源插件。点「获取 API Key」时先显示目标域名再打开。
- 导出字幕的格式下拉框、学习导出的目标列表里出现插件项，后面标出插件名。
- 插件命令出现在命令面板和菜单栏。在 macOS 上，不认识的分组会由现有逻辑放在「窗口」菜单前面，单独成一个菜单（`menu.go:20`）；在 Linux 和 Windows 上，命令面板是唯一入口。

### 7.5 错误

- 插件失败不弹窗打断，在出错的地方说清楚，比如查词卡上写「JMdict 没有这个词，问了 AI」。
- 错误码在 `plugins` 包里是英文哨兵错误（`ErrUnknownKind`、`ErrUnsupportedKind`、`ErrBadSignature`、`ErrRevoked`、`ErrTampered`、`ErrUnknownKey`、`ErrTooLarge`、`ErrBadID`、`ErrNoSecrets`、`ErrHung`……），由 `bind_plugins.go` 映射成 catalog 文案，不把 `err.Error()` 拼进本地化句子。

### 7.6 多语言

- 看懂自己的文案按 AGENTS.md 写进 `apps/kando/locales` 的五份 catalog，前缀用 `settingsPlugins…`、`plugins…`，然后跑 `go tool nsgo i18n`，把 `catalog_gen.go` 一起提交。
- 插件提供的文字来自清单的 `Text` map，只在 `LabelFunc`、`ContentFunc`、`core.Dynamic` 这类绑定里用 `Text.In(i18n.Language(cx))` 解析（NSGo 的语言是字符串，见 `i18n/i18n.go:104`），这样切换语言时会跟着变，满足 `locales/follow_test.go`。找不到当前语言就沿 `i18n.Parent` 回退，最后用 en。
- 复数只支持 `i18n.Cardinal` 的 one/other，第一版不做。
- 插件文字一律当纯文本显示，并限制长度。
- 手机端：不显示插件，只同步插件的产出（glossary 已经体现在字幕里，标注和读音作为普通数据）。查词结果经 `vocabResolve` 由桌面算好，回给手机。不加新的 `Platform` 值，不改协议版本。

## 8. 代码布局

按 AGENTS.md，插件系统是一个新的块：纯逻辑放进非界面子包，`package main` 里只放 `app` 方法，每个共享文件只加一行。

### 8.1 新增的非界面包（不导入任何界面包）

- `apps/kando/plugins/`
  - `manifest.go`：Manifest、Kind、Text、Parse（Credential 在 P3 加入）
  - `validate.go`：Validate、id 正则、包形状、https 规则、`ErrUnsupportedKind`；发布者段与证书的对应、capabilities 只许 Host 和 Network 在 P3 加入
  - `install.go`：Stage（按字节计数、嵌套 zip、Lstat 复制）、Swap、Remove、Scan、清理残留、路径 `IsLocal` + `Rel` 检查
  - `hash.go`
  - `state.go`：state.json（单写者，tmp 加 rename）；P1 只有 `{id, hash, enabled}`，Grant 和子集比较在 P3 加入
  - `health.go`：health.json 和 in-flight 标记（P3）
  - `audit.go`（P3）
  - `perm.go`：Perm 常量和请求、响应类型（P3）
  - `errors.go`
  - `names.go`：清洗、长度限制、保留名单
  - 测试跟着进包
- `apps/kando/plugins/dict/`：解析 Yomitan、CC-CEDICT、TSV，安装时生成排序索引 `index.bin`，查词时在磁盘上二分查找，不整份读进内存，用完可以 Close。
- `apps/kando/plugins/wordlist/`：解析词表，word → tier。
- `apps/kando/plugins/script/`（P3）：
  - Trust：OpenTrust，每次启动 AddRoot 内嵌根和 dev 根，吊销。
  - Verify，预读沙箱里比对 `kandoManifest()`。
  - `Limits()`：以 `tsx.DefaultLimits` 为底。
  - 构造 Sandbox 的唯一函数 `script.Sandbox(id, signer string, grant plugins.Grant, host *core.Host) tsx.Sandbox`：Grant 从零构造成 `&pkg.Capabilities{Network, Host}`，Storage 是 `plugin.<id>.<signer>`，Limits 来自 `Limits()`。
- `packages/media/translate.go`（P2.0）：`ChatWith`、`ChatRequest`、`ChatReply{Text, Usage}`；`Chat` 改成调它，行为不变。
- `packages/core/segment`：新增 `Annotated(lang, line) []Morph`（P1.2）；完整 features 词典按需加载，`Words` 不变。
- `packages/core/videosites_validate.go`（P4.0）：`ValidateVideoSite`，返回 error，CLI、测试和运行时共用。原来 `browse/videos_test.go:89` 的 `shared` 正则挪到这里导出。

### 8.2 根目录 package main 的新文件

- `state_plugins.go`：pluginsState、savedPlugins、pluginsHooks、pluginKeys、pluginsOverlay、pluginTerms、PluginPresets、llmPresets、pluginCall。
- `bind_plugins.go`：settings.App 和 film.App 需要的插件 getter 和 action（`live.Func`、`live.Do`），错误到 catalog 的映射，每个插件自己的 `core.Host` 和 handler（读 app 状态、防剧透裁剪、LLM 代理、credentialFetch）。
- `plugins_test.go`、`screenshot_plugins_test.go`。

### 8.3 共享文件（各一行）

- `main.go`：`app` 加字段 `plugins pluginsState`，`newApp` 加一行。
- `settings_file.go`：`savedSettings` 嵌入 `savedPlugins`，`clean` 加一行。
- `events.go`：`areas()`（`:54`）在 settingsHooks 之后加 `a.pluginsHooks()`。
- `keys.go:83`：`slices.Concat` 加 `a.pluginKeys(cx)`（P3）。
- `state_overlay.go:23`：`overlays()` 加 `a.pluginsOverlay(cx)`。

### 8.4 各接点的最小改动

- `settings/presets.go`：`PresetByID` 和 `ProviderID` 改成接收预设列表（P0.1），连同 E4 列出的全部调用处；`settings_ai.go:47` 的 clean（P0.2）。
- `state_vocab.go:215` 的 LookUpWord 和 `state_phonesync_study.go` 的 `phoneStudy.LookUp` 都改成调 `vocabResolve`（P1.3）。
- `state_glossary.go:119`、`:151` 的 `FilmTerms`（P1.1）。
- `bind_settings.go:189` 的 `Presets()` 改成返回 `a.llmPresets()`，设置 › AI 的下拉框按来源分组（P1.1）。
- `packages/core/segment` 的 `Annotated`（P1.2）。
- `film/subtitle.go:269` 的 lineWords 加 marks（P1.4），`film/subtitle` 叶子包里加 `Mark`。
- `settings/sections.go:32` 改成函数，`settings/window.go` 的 `page()` 在 default 分支照 `Window.Account` 的先例注入，`settings/sectionid` 加 `Plugins`。
- `packages/media/translate.go` 的 `ChatWith`（P2.0）。
- `subexport` 的格式列表（P3）。
- `saved/targets.go`（P2）。
- `cli/cli.go:148-165` 加一行。
- `remote/remote.go` 和 `remote_api.go` 加只读的 `plugins.list`。
- `state_browse_buttons.go` 的 `browsePageAsk`、`browsePageAdd` 接 `pluginSites`，队列构造站点包任务时置空 Cookies（P4）。
- `locales/*.yaml` 五份，加上 `catalog_gen.go`。
- `film/panel.go:26`（P5）。
- 不改：`packages/media` 的 Pipeline、Run、Tools 语义（P5 的 Engine 字段除外）；`sites/catalog` 和 `kcore.VideoSites`；`state_browserext.go` 的判断。

## 9. 测试策略

- **`plugins` 包单元测试**：
  - 解析：未知键、未知 kind、schema 过新。
  - id 规则：`.`、`..`、`a..b`、`-a`、`a.`、不含点的 id 一律拒收；`kando.` 保留前缀；拼出的路径必须落在 `root/<kind>/` 下。
  - 包形状：zip-slip、符号链接、绝对路径、超大小和超条目数、未被引用的文件。
  - zip bomb：`UncompressedSize64` 谎报的条目、嵌套 zip 超额，都应被字节计数器拦下。
  - 文件夹安装：校验后再改原目录，安装结果不受影响（TOCTOU）。
  - Capabilities：签名 Capabilities 里带 `secrets`、`openUrl` 等字段的包被拒；network 规则缺 methods 或 paths 被拒；ai-service 的非回环 http 被拒；保留名（含内置服务商名）被拒。
  - 清洗和哈希：Bidi 和零宽字符清洗、哈希稳定性。
  - 安装事务：staging 中途失败不留下半个包；同 id 替换遵循 5.1 节的顺序。
  - state.json：遇到未知 format 时视为「全部未启用」。
  - 未实现的 kind（`script`、`site` 等）被 `Validate` 以 `ErrUnsupportedKind` 拒收，不报未知键。
  - Grant 子集比较（P3）：只扩 credential 的 hosts、只改 header 或 format、只加一条 network 规则，都应判为扩大能力；换签名者时授权作废。
- **对解析器做模糊测试**：`go test -fuzz` 覆盖 `Parse` 和 zip 解包，跑在 CI 里，带 `-fuzztime` 上限。
- **`dict` 和 `wordlist`**：用小型样例词典验证索引构建、二分查找、原形查询、CC-CEDICT 的拼音，以及 Close 之后 Swap 能成功。
- **`segment.Annotated`**：`行った` 得到原形 `行く`；`Words` 的输出与改动前逐字相同。
- **`core.ValidateVideoSite`**：拒绝嵌套量词、反向引用、环视。
- **假脚本插件**：测试辅助函数在测试里现场生成测试根密钥，用 `pkg.Build` 签一个小 TSX bundle（用 `tsx/bundler.Compile` 或预编译好的 JS 字符串），不提交二进制夹具。准备这些夹具：
  - 正常插件：导出 lookUp、run、exportSubtitles；
  - 死循环插件：验证 `ErrBudget` 和熔断；
  - 异步等待插件：`await host.call` 慢返回，验证等待不计入预算；
  - 越权插件：调用未声明的 handler，应收到 `denied`；
  - 篡改插件：清单和 `kandoManifest()` 不一致，应拒装；
  - 顶层写存储插件：预读之后 probe 分区被删，`App.ID` 分区里也没有数据；
  - 被吊销插件；
  - 回滚插件：build 号更低，应被拒绝；
  - 证书范围不符的插件：id 的发布者段与证书的 `Apps` 不一致，应被拒绝。
- **安全不变量测试**：
  - 用 `go/ast` 扫描整个仓库，断言 `tsx.Sandboxed` 一处都不出现；`tsx.Load`、`tsx.View` 只出现在 `plugins/script`，并且都经过 `script.Sandbox`。
  - `script.Sandbox` 构造的 Grant 里，除 `Network` 和 `Host` 外所有字段都是零值（用反射遍历 `pkg.Capabilities` 的字段断言）。
  - Kando 构造的每份 Limits（运行时和预读）都满足 `Harden == true`，`Stack`、`Nodes`、`Timers`、`Requests`、`Body`、`Storage` 都大于 0。
  - 每个插件拿到的 `*core.Host` 都是不同的实例。
  - 插件 JS 用 `nsgo.Secrets` 读不到 `pluginhost.` 分区的凭据。
  - 任何 handler 返回的 JSON 里都不出现 `baseURL`、`apiKey` 和绝对路径。
  - `credentialFetch` 对未授权的 host、method、path 不注入，也不跟随重定向。
  - 预设解析：`ProviderID(a.llmPresets(), base)` 对插件地址返回 `pack.<id>`。
  - in-flight 自愈：预先放一条残留的 in-flight 记录再启动，那个插件被标为 `hung` 并停用。
  - `llm.chat` 的 `ErrHostTimeout` 不计入熔断。
- **app 级测试（nsgotest）**：`newTestApp(t, p)` 在 `f.home` 下放样例包。
  - 用 `settingsOpenAt(a, tt, sectionid.Plugins)` 进入设置页，验证列表、开关、安装确认（含高风险组合和凭据主机行）、问题区和错误文案。
  - 离线、无 AI 时，在桌面点词出现词典释义；手机同步路径（`phoneStudy.LookUp`）同样返回词典释义；词典顺序生效。
  - glossary 包的条目不覆盖用户的 Locked 条目，`FilmTerms` 也包含包的条目。
  - 选择 pack 预设后，服务确实存在 `pack.<id>` 下；拿掉包再跑 `savedLLM.clean`，key 还在；`savedPlugins.clean` 保留缺失的 id。
  - 标注只在 `playing` 发现新的影片键时计算一次，任务进度触发的 `shelfChanged` 不会让它重算。
  - `quitStarting` 之后所有插件都已 Close。
  - 插件命令进入菜单和面板，和内置按键冲突时插件的按键被清空（运行时去重，和 `keys_test.go:266` 的唯一性测试并存）。
  - 切换语言后插件名跟着变（follow_test 的规则）。
  - 在 CLI inbox 放一个文件，app 只弹确认，不直接安装。
  - P4：`browserExtVideo` 和 remote 不认插件站点；站点包任务的 Tools 不带 Cookies。
  - 现有的 `settings_file_test`、`locales/catalog_test` 照常要求：五份 catalog 齐全、settings 键唯一、没有自定义 Marshal。
- **截图**：`screenshot_plugins_test.go` 的 `TestScreenshotsPlugins` 覆盖插件列表、安装确认、详情页、查词卡来源标注、带注音和等级色的字幕。样例包放在 `f.home`，路径显示成 `~/…`，不读本机环境。
- **P3 spike 的性能和安全门槛**：
  - 量出链接 goja 前后三平台的二进制大小差；
  - 在 CI 上跑三平台的空插件 Load 和 Call，测「引擎 goroutine 单次阻塞的最长时间」和 in-flight 写盘的开销；
  - 用 ReDoS 正则和大分配的夹具，确认上游 issue 修好之前的实际行为（会卡多久、内存涨到多少），连同 in-flight 自愈一起报给 owner。

## 10. 分阶段落地计划

每一期都能单独发布，前一期不依赖后一期。PR 粒度按「一件事一个 PR」。改动超过 10 个文件的 PR 先把计划给 owner 看。

### P0 地基（用户看不到变化）

- PR 0.1：`PresetByID`/`ProviderID` 改成接收预设列表，所有调用处一起改（列表见 8.4），行为不变，加测试。涉及约 13 处调用、6 个文件。
- PR 0.2：`savedLLM.clean` 保留 `pack.` 前缀的服务，加测试。
- PR 0.3：`apps/kando/plugins` 骨架：Parse（两步解码）、Validate（id 正则、路径检查、包形状、本轮四种 kind 的字段规则，其余 kind 返回 `ErrUnsupportedKind`）、Stage（字节计数）、Swap、Remove、Scan、最小的 state.json（`{id, hash, enabled}`），加上 CLI 的 `kando plugin validate`。app 还不加载任何包。health.json、in-flight 标记和 audit.log 只服务脚本插件，推到 P3.1（12.2 第 4 条）。

### P1 数据包 v1

- PR 1.1：设置 › 插件页、安装确认、拖入安装、CLI inbox，加上 glossary 和 ai-service 两种 kind（接入 E3 的两处和 E4 的 `llmPresets`、https 规则、下拉框分组），以及 savedPlugins、pluginsHooks（restore/save）、五份 catalog、截图。这是第一个对用户可见、范围最窄的 PR。
- PR 1.2：`segment.Annotated`。先在 Ubuntu 编译机上量出 `ipa.Dict()` 相对 `DictShrink()` 的 RSS 增量和加载时间，按 12.2 第 5 条的门槛选用完整 features 词典或退到「只用词典包」的方案（见 E1），数字和选择写进第 15 节。不改 `Words` 和 `WordToken`。和 PR 1.3 同在 `feat/plugins-dictionary` 分支。
- PR 1.3：dictionary kind，含 `plugins/dict`、`vocabResolve`、`LookUpWord` 和 `phoneStudy.LookUp` 都接入、查词卡来源标注、词典排序。
- PR 1.4：wordlist kind，加上注音和拼音标注（E2，按 `playing` 懒算），以及设置 › 外观 的开关。
- ~~PR 1.5~~：官方包 CC-CEDICT、JLPT、HSK 随 app 分发。不做，词典包由用户自己安装（12.2 第 2 条）。
- 验收：装好词典包和词表包后，断网、不配 AI 时，在桌面和手机上点日语或中文的词都能看到释义，字幕上有注音和等级色。

### P2 学习任务与导出模板

- PR 2.0：`packages/media` 的 `ChatWith`（MaxTokens、Header、Usage），`Chat` 改成调它，行为不变，加测试。
- PR 2.1：把五处重复的异步 Chat 流程合并成 `runTask`（explainAsk、LookUpWord 的 Chat goroutine、recap、palette ask、correctCandidates），先补测试，行为不变。
- PR 2.2：task kind，加每日 token 预算（usage 或字符估算），接入命令面板。
- PR 2.3：translate-style kind。
- PR 2.4：`saved/targets.go` 注册表，加上 export-template kind。
- PR 2.5：应用内「把我的术语表或词表导出成包」，同一种格式，走同一个校验器。

### P3 脚本插件（只在桌面端，先过 spike 门槛）

- PR 3.0（spike，不合并，或者只合并测量脚本）：链接 nsgo/tsx，量出三平台的体积增量、引擎阻塞时间、in-flight 写盘开销，以及 ReDoS 和大分配的实际表现，结果报给 owner，由 owner 决定是否继续。不达标就停在 P2，数据包的价值不受影响。
- PR 3.1：`plugins/script`：Trust（每次启动 AddRoot）、Verify、预读沙箱比对 kandoManifest、`Limits()`、Sandbox 构造、熔断和 in-flight 自愈（health.json），audit.log；`State` 的每一项加上 `Grant`、`Signer`、`OldHash`、`Budget`，`State.Format` 升一级；加上 Grant、Limits、Sandboxed 的不变量测试和假插件夹具。
- PR 3.2：每个插件自己的 host 和 handler（film、line、lines.seen、saved、words、sound、toast、settings.get），`llm.chat` 代理和预算（用 `ChatWith`），事件推送，逐个 update 载入。
- PR 3.3：命令贡献（`keys.go:83`），加运行时按键去重。
- PR 3.4：查词来源、导出格式（`Export.Cues`）、学习导出目标、标注器。
- PR 3.5：`credentialFetch`（methods 和 paths 限制、`pluginhost.` 分区、host 表警示），加上 secret 字段的设置渲染。
- PR 3.6：签名运维：插件根密钥、`go tool nsgo certify/revoke` 的发布流程、吊销列表随 appcast 发布、app 内的开发者模式（生成密钥、打包并安装文件夹）、CLI 的 `kando plugin list|validate|install`。
- PR 3.7：官方参考插件 `kando.ankiconnect`，同时作为示例和 CI 夹具。

### P4 站点包

- PR 4.0：`core.ValidateVideoSite`。
- PR 4.1：site kind 和单独的 `pluginSites` 匹配器，只接内置浏览器的两个入口，逐域名同意，不带浏览器 cookie，不支持 sign-in。加测试锁住 socket、浏览器扩展、remote 不认插件站点。设置页说明浏览器扩展不覆盖插件新增的站点。

### P5 按需单独立项

各项单独评审，不承诺：

- 候选译文和机器翻译来源（`media.StageError` 加 `Tools.Engine`）；
- 播放器侧栏的「扩展」标签和 `tsx.View`；
- 社区索引（R2，数据包限速后即时上架，代码包审核后上架，安装仍然不授予任何能力）；
- TTS。

## 11. 新依赖

不新增任何 module。用到的都已经在 `apps/kando/go.mod` 里：

| 模块 | 版本 | 用途 | 位置 |
|---|---|---|---|
| `github.com/nsgo-dev/nsgo`（第 13 行，直接依赖） | `v0.1.1-0.20261118094851-ec272782be28` | P3 起新用到 `tsx`、`tsx/pkg` 两个子包，用于脚本运行时、签名、Trust、吊销；`i18n` 用于插件文案的回退；`core.Secrets` 存 dev root 和插件凭据 | `apps/kando/plugins/script`、`bind_plugins.go` |
| `github.com/dop251/goja`（第 46 行，目前 indirect） | `v0.0.0-20261027234916-5f913d1693d9` | 被 tsx 链接进二进制。不是新 module，但会让二进制变大，P3.0 先测量并报给 owner | 间接 |
| `github.com/ikawaha/kagome-dict/ipa`（已在用） | 现有版本 | P1.2 额外按需加载完整 features 的 `ipa.Dict()`，内存开销先测量 | `packages/core/segment` |
| 标准库 `encoding/json`、`archive/zip`、`crypto/sha256`、`sort`、`path/filepath` | Go 工具链 | 清单严格解码、包解压、内容哈希、词典索引、路径检查 | `apps/kando/plugins`、`plugins/dict` |

`packages/media` 新增的 `ChatWith` 是自己的代码，不涉及依赖。

刻意不用的：

- `go.yaml.in/yaml/v3`：已经固定在 go.mod 里，但清单统一用 JSON。
- TOML 库、wazero、gopher-lua、hashicorp/go-plugin、sqlite（词典索引用自己的排序文件加二分查找）。
- go-sdk 的 MCP 客户端：外部工具层已否决。

如果后续确实需要新依赖，会先列出模块、版本、用途和放在哪个模块，向 owner 确认后再写进 go.mod。

## 12. 风险与开放问题

### 12.1 风险

1. **tsx 和 goja 在看懂里还没跑过**：三平台的成熟度、二进制体积和 goja 内存都没有测过。缓解：P3.0 spike 设门槛，数据包两期不依赖它。
2. **插件跑在引擎 goroutine 上，而且卡死不一定能打断**：await 不计入预算（`sandbox.go:311` 的 `within`），同步片段有 500ms 预算；但 regexp2 的回溯匹配不响应中断，Limits 也没有内存上限，恶意插件可以让窗口永久卡死或耗尽内存（6.7 节）。缓解：in-flight 记录加下次启动自动停用，签名问责，重计算（分词、注音）留在 Go 原生实现，Load 逐个 tick 进行；上游已有 [nsgo-dev/nsgo#759](https://github.com/nsgo-dev/nsgo/issues/759)（正则不可中断，已复现：200ms 发 Interrupt，10s 后仍在跑）和 [nsgo-dev/nsgo#760](https://github.com/nsgo-dev/nsgo/issues/760)（没有内存上限）。如果 P3.0 测出引擎阻塞问题，再请求让无 UI 的插件跑在 worker goroutine 上。
3. **签名元数据缺口**：靠比对 `kandoManifest()` 绕行，上游是 [nsgo-dev/nsgo#761](https://github.com/nsgo-dev/nsgo/issues/761)（见 4.3）。
4. **`tsx.Sandboxed` 和共享 Host 才是绕开权限的入口**：`tsx.Sandboxed` 在 Capabilities 和 Grant 都为 nil 时拿到宿主的全部权限，连 `file://` 都能读；Load 路径上的 nil Grant 会让包声明的能力绕过用户勾选生效；handler 只要用 `a.cx` 调 NSGo 服务，就带着 app 的权限。缓解：AST 测试全仓禁止 `tsx.Sandboxed`；Grant 由唯一构造函数从零构造，并测试零值字段；每个插件独立的 Host；每个 handler 都查 `Effective`。
5. **site 包**：已经改成独立的 `pluginSites`，不放宽全局闸门；剩下的风险是 JS 正则的校验漏了嵌套量词时会让页面卡顿，以及用户对「浏览器扩展不覆盖插件站点」的预期要讲清楚。
6. **clean 丢数据**：`savedLLM.clean` 和 `savedPlugins.clean` 一旦写错，插件暂时缺席时就会丢用户的 key 或选择。P0.1、P0.2 和 P1 都有测试锁住。
7. **token 费用**：task 和 `llm.chat` 花的是用户的钱，默认预算必须保守，用量必须可见；服务商不返回 usage 时只能估算。
8. **离线用户收不到吊销列表**：长期离线时，已被吊销的纯本地插件可能继续运行，这是有意的折中。本机程序还可以通过删改 trust.json 清掉吊销和防回滚（6.7 节）。
9. **凭据**：Linux 没有 Secret Service 时，凭据类插件不可用；Windows 和 Linux 的凭据库对同用户开放（6.7 节）。
10. **i18n**：插件文案不经过 `catalog_test` 检查，缺语言时回退到 en，翻译质量取决于作者。
11. **手机版本滞后**：插件产出的数据同步到手机时，ledger 必须向后兼容。
12. **P2.1 的重构**会碰 5 个以上文件，要先给 owner 看计划。P0.1 按 E4 列出的调用处直接做，行为不变，由测试锁住。
13. **P1.2 的内存**：完整 features 词典可能明显增加常驻内存，所以先测量，达标（12.2 第 5 条）才只在桌面按需加载，否则走只用词典包的退路。

### 12.2 已定

由 owner 的代理在 2026-11-19 决定：

1. **本轮只做 P0（0.1、0.2、0.3）和 P1 的 1.1–1.4**：1.1 是设置 › 插件页、安装和 glossary、ai-service 两种 kind；1.2 是 `segment.Annotated`；1.3 是 dictionary；1.4 是 wordlist 和逐词标注。理由：这几项不执行第三方代码，对学习者价值最高，能单独发布，也不依赖还没决定的根密钥保管。P2、P3、P4、P5 推迟。
2. **P1.5（随 app 分发官方词典包）跳过，词典包由用户自己安装**。理由：JMdict、CC-CEDICT 是 CC-BY-SA，随包分发要先过法务；官方包还会让安装包明显变大。设置 › 插件页和查词卡的提示引导用户「从文件安装…」。
3. **本轮实现的 kind 只有 glossary、ai-service、dictionary、wordlist**。其余 kind 解析器认得，`Validate` 用 `ErrUnsupportedKind` 拒收，到各自那一期再放开（第 3.1、4 节）。理由：不给还没做的阶段留死代码；旧版本遇到新 kind 时也有明确、可本地化的报错。
4. **P3 才有的东西本轮一律不写**：不导入 `nsgo/tsx`，不定义 `Grant`、`Perm`、`Credential`，不做 health.json、in-flight 标记和 audit.log。`state.json` 的每一项只有 `{id, hash, enabled}`，`Grant`、`Signer`、`OldHash`、`Budget` 到 P3 再加（5.1 节）。理由：同上，避免没有调用方的代码；`State.Format` 保留，P3 升格式时旧文件按 6.2 节的规则处理。
5. **P1.2 的选择按测量结果定**：在 Ubuntu 编译机上（`ubuild`）量完整 features 的 `ipa.Dict()` 相对 `DictShrink()` 的常驻内存增量（RSS）和首次加载时间。增量不超过 200 MiB、加载不超过 3 s，就在桌面上、只在启用了日语的词典包或词表包时按需加载完整词典；否则用 E1 的退路：Annotated 不出 Base 和 Reading，读音只取自词典包。理由：原形和读音对查词命中率帮助很大，但不能让所有用户为它付内存；手机的分词始终不变。测量数字和最终选择写在第 15 节。
6. **看懂自己的 LLM key 留在 settings.json 原处**，本轮不迁进 `core.Secrets`。理由：与插件系统无关，本轮的数据包也碰不到它；以后是否迁移见 12.3。
7. **不新增任何 Go module**。确实需要时停下来报告，不自行加入 go.mod。
8. **NSGo 的问题已提交上游**：[nsgo-dev/nsgo#759](https://github.com/nsgo-dev/nsgo/issues/759)（tsx 的回溯正则不响应 Interrupt，已复现：200ms 发 Interrupt，10s 后仍在跑）、[nsgo-dev/nsgo#760](https://github.com/nsgo-dev/nsgo/issues/760)（`tsx.Limits` 没有内存上限）、[nsgo-dev/nsgo#761](https://github.com/nsgo-dev/nsgo/issues/761)（`pkg.Contents` 不支持 app 自定义的签名条目）。三者都只影响 P3，本轮不需要绕行。

### 12.3 仍待 owner 决定

1. **插件根密钥由谁保管，是否开放第三方证书**：离线介质、备份份数、轮换策略；是否开放第三方「已验证」证书，审核门槛（含 network 规则必须最窄）是什么。这是 P3 的前置条件；决定之前，P3 只跑官方插件和开发者模式插件。
2. **开发者模式的细节**：是否允许用户本机 dev root 侧载（默认建议允许，开着时一直显示徽标）；dev 包能否覆盖、何时过期；是否做 trust.json 的回退检测（6.4 节的可选加固）。
3. **P3.0 spike 的放行门槛**：二进制体积增量上限、引擎单次阻塞上限，以及上游还没修 #759、#760 时，是否接受「卡死后下次启动自愈」作为过渡；默认每日 token 预算（建议每个插件 50k）。
4. **是否做站点包（P4）**。
5. **社区索引**（P5）：做不做，代码包审核由谁负责。
6. **是否把看懂自己的 LLM key 迁进 `core.Secrets`**：与插件无关的加固，作为独立 PR，Linux 上需要保留明文退路。
7. **外部原生工具或 MCP 子进程插件**：本设计否决了。如果以后识别（faster-whisper）、下载器确有需求，需要单独立项，前提是先给 `media.Command` 补上进程组或 Job Object、WaitDelay 和整棵进程树的 kill。顺带一提，这三项的缺失本身就是一个与插件无关的现有风险：任务取消时，yt-dlp 启动的 ffmpeg 孙进程不会被一起杀掉，`runFollowing` 可能卡在 `cmd.Wait`。这一点是读代码推断的，没有复现，建议单独开 issue。

## 13. 评审记录

阻断问题：

- **Limits 字面量会关掉 Harden、让配额变成无限**（两条评审指向同一处）：3.3 节说明零值的语义，运行时的 `script.Limits()` 和 4.3 节的预读沙箱都以 `tsx.DefaultLimits` 为底再覆盖字段；禁止节点写 `Nodes = 1`；预算改为 Load 1s、Call 500ms；第 9 节加了 Harden 和各配额大于 0 的不变量测试。
- **E4 只在调用方追加预设行不通**：`PresetByID`/`ProviderID` 改成接收预设列表，app 提供 `llmPresets()`，约 12 处调用列进 8.4 节，作为 P0.1 单独成 PR；clean 的测试改成针对真正存在 `pack.<id>` 下的服务。
- **kagome 的 features 被裁掉，gse 没有读音**：E1 和 P1.2 改成按需加载完整 features 的 `ipa.Dict()`（只在桌面、并且先测量内存），不达标就退到只用词典包的方案；合并词取首个 token 的原形；删掉「gse 提供读音」的说法；按 12.2 第 5 条的测量门槛选择。

主要问题：

- **「goja DoS 已被 Limits 兜住」不成立**：6.7 节如实写出 regexp2 卡死和 OOM；加了 in-flight 记录和下次启动自动停用（5.2、6.6 节）；两个 NSGo 问题已提交为 #759、#760；P3.0 的门槛算进这一项。
- **凭据 hosts 由作者决定，更新时可以悄悄扩大**：授权改成存完整的 `Grant`（权限、network 规则、凭据的 hosts/header/format/methods/paths），更新时逐项做子集比较，有凭据变化就暂停注入；输入框上显示「此密钥只会发送到」；加了内置服务 host 表的警示。
- **「提示注入无法自动产生后果」是错的**：6.2 节改成「网络能力本身就是副作用通道」；高风险组合单独标出；network 规则和凭据必须写死 methods 和 paths；AnkiConnect 端点的残余风险如实写进审核规范和详情页。
- **id 能写成 `..`，存在路径穿越和 zip bomb**：id 改用新的正则，加 `IsLocal` 和 `Rel` 检查；解压按实际字节计数，嵌套 zip 共用计数器；文件夹安装改为 Lstat 复制进 staging 后校验副本。
- **macOS 上让同用户程序更容易拿到凭据**：Storage 和凭据分区都带上签名者 key ID，签名者变化时清空并要求重新输入；dev root 公钥存进 `core.Secrets`；开发者包不能占用非开发者包的 id；6.7 节分平台写明凭据库的差异。
- **站点包会放宽全局闸门，还会带上 cookie**：改为独立的 `pluginSites`，只接 `browsePageAsk` 和 `browsePageAdd`；`browserExtVideo` 和 remote 明确不认，并写测试锁住；站点包的任务不带 `--cookies-from-browser`；`catalog` 和 `VideoSites` 不再改动。
- **手机查词不经过 LookUpWord**：抽出共用的 `vocabResolve`，桌面和手机都调它，词典放在 `s.ask(f)` 之前；`state_phonesync_study.go` 列进 P1.3 和 8.4 节。
- **nil Grant 的语义写错，共享 Host 的边界没讲**：3.3 节和风险第 4 条改成准确的描述；AST 测试全仓禁止 `tsx.Sandboxed`；加了「每个插件独立的 Host」「handler 不在 `a.cx` 上替插件调服务」两条不变量；Grant 从零构造，并测试零值字段。
- **`quitting` 时已经不能 Close**：改在 `quitStarting` 里 Close，`quitting` 不用。
- **每日 token 预算、maxTokens、X-Title 在现有 Chat 上做不到**：新增 P2.0 的 `media.ChatWith`；服务商不给 usage 时按字符估算；`packages/media` 列进 8.1 和 8.4 节，非目标一节相应修改。
- **dev root 重启后丢失，CLI 拿不到 Secrets**：app 每次启动都重新 AddRoot 内嵌根和 dev 根；密钥生成和开发者签名移进 app 的设置 › 插件 › 开发者；CLI 和 socket 都不签名。
- **证书的 app-ID 范围和 id 规则对不上**：id 改为 `<publisher>.<name>`，Validate 检查发布者段与证书一致；示例改名为 `kando.ankiconnect`、`example.jmdict-en` 等。

次要问题（都已处理）：

- ai-service 钓鱼：https 规则、内置服务商名加入保留名单、下拉框分组、打开 keyURL 前显示域名。
- Grant 白名单和凭据分区：从零构造 Grant，凭据移到 `pluginhost.` 分区。
- 预读时模块顶层代码执行：用一次性分区，Close 后立即删除，Limits 收紧。
- trust.json 对同用户可写：写进 6.7 节，并在 6.4 节给出可选加固。
- 启动时同步 Load：改为每个 update 只载一个插件，删掉「hook 顺序让菜单已有插件行」的理由。
- 缺 `save` 钩子、`shelfChanged` 触发标注不合适：加了 `save`，标注改由 `playing` 按「视频 + 字幕哈希」懒算。没有采用「在 film 块的打开函数里调用」，因为 AGENTS.md 不允许往别的块的函数里插调用。
- 错误的锚点：E3 改锚到 `FilmTerms`，E6 改用函数名。
- 30s 的 host.call 超时：要求插件传 `timeout`，宿主侧 120s 取消，`ErrHostTimeout` 不计入熔断。
- `Text.In` 的参数和 network 的形状：`In` 改为接收 string，manifest 改用 `pkg.Capabilities` 的 JSON 形状，Network 也纳入比对。
- 替换同 id 包时 rename 失败：在 5.1 节写明「关句柄 → 旧目录改名 → 新目录就位 → 重新打开」的顺序，以及 Windows 上的重试和延后替换。

## 15. 实现状态

| PR | 范围 | 分支 | 状态 |
|---|---|---|---|
| P0（0.1–0.3） | 0.1 `PresetByID`/`ProviderID` 接收预设列表；0.2 `savedLLM.clean` 保留 `pack.` 服务；0.3 `apps/kando/plugins` 骨架（Parse、Validate、Stage、Swap、Remove、Scan、最小 state.json）和 `kando plugin validate` | `feat/plugins-foundation` | 进行中 |
| P1.1 | 设置 › 插件页、安装确认、拖入安装、CLI inbox，glossary 和 ai-service 两种 kind，savedPlugins、pluginsHooks | `feat/plugins-data-packs` | 未开始 |
| P1.2 + P1.3 | `segment.Annotated`（含测量和路线选择），dictionary kind、`plugins/dict`、`vocabResolve`、查词卡来源、词典排序 | `feat/plugins-dictionary` | 未开始 |
| P1.4 | wordlist kind、`plugins/wordlist`、逐词标注（注音和等级色）、设置 › 外观 的开关 | `feat/plugins-wordlist` | 未开始 |
| P1.5 | 官方词典包 | — | 不做（12.2 第 2 条） |
| P2 | 学习任务、导出模板、翻译风格 | — | 未开始 |
| P3 | 脚本插件 | — | 未开始（等 12.3 第 1、3 条） |
| P4 | 站点包 | — | 未开始（等 12.3 第 4 条） |
| P5 | 按需单独立项 | — | 未开始 |

P1.2 的测量结果（Ubuntu 编译机，`ipa.Dict()` 相对 `DictShrink()`）：待测。

与正文的偏差：实现过程中某个接点挪了位置或做了简化，就同时改正文对应的一节和这一段，写明改了什么、为什么。目前没有偏差。
