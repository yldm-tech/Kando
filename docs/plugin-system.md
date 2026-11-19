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
- 所以先抽出一个共用的 `func (a *app) vocabResolve(q lookup.Query, film string) vocabAsk`（P1.3 已实现，在 `state_vocab.go`），顺序是缓存 → 已启用的词典（按 `savedPlugins` 里的顺序）→ AI。它分两半：`vocabResolve` 在引擎 goroutine 上读缓存、挑出已打开的词典、取 AI 服务；后台的那一半不碰 app 状态，放在非界面包 `lookup` 里（`lookup.Ask.Answer(ctx)`，词典按 `lookup.Dictionary` 这个窄接口传入，`*dict.Dict` 满足它，`lookup` 不导入 `plugins/dict`），查词典、再问 AI；决定按哪种语言挑词典的 `lookup.WordLanguage` 也在那里。`LookUpWord` 和 `phoneStudy.LookUp` 都改成调它，词典在 AI 之前，手机那条路不再先调 `s.ask(f)`，只有词典都没有这个词、又没有能用的 AI 服务时才回 `psync.CodeNoAI`，所以离线的手机也能查到词。
- 缓存只存 AI 的回答：词典的结果每次现查（本地、很快），不写进缓存，免得挤掉花钱、要联网才能重新拿到的 AI 回答（缓存上限 5000 条）。手机只读（`psync.Caller.ReadOnly`）时的规则不变：它的 AI 回答也不写进缓存。
- 挑词典：词的语言取影片的说话语言（`Spoken`），没有时看文字（有假名是日语，只有汉字时看整行有没有假名，否则是中文），都看不出就不按语言筛。词典的 `from` 要和它是同一种语言；`to` 和回答语言（`core.TargetTag(q.Language)`）相同的词典排在 AI 之前；`to` 是别的语言的词典只在没有能用的 AI 服务、或 AI 服务出错没答上来时才答（比如目标语言是简体中文、只装了 JMdict 英文版，离线时照样能看到英文释义，有 AI 时由 AI 用中文答）。
- 查词卡在释义下面写出来源：「来自词典「<名称>」 · <attribution>」或「由 AI 服务解释」（缓存里的也是 AI 的回答）。手机也一样：手机同步协议 v2 的 `psync.WordEntry` 加了可选的 `Source`（`psync.WordSource`，`kind` 是 `dictionary` 或 `ai`；词典带名称和 attribution，用桌面的界面语言，`phoneWordSources` 在引擎 goroutine 上按包现取；`desktop` 是回答的这台电脑的名字（`phoneSyncName`），手机把它和查词结果一起存下，之后从手机自己的缓存里答时也写出当初回答的电脑，而不是当前影片所在的电脑，没有名字时写「由电脑上的 AI 服务解释」），手机的查词卡在释义下面照桌面的写法显示（`apps/mobile/player/words.go` 的 `sourceText`）。它是 v2 里新增的可选字段（`omitzero`），旧手机解 JSON 时忽略它，旧桌面不发它、手机就不显示来源；`packages/sync/v2.go` 的协商规则是新增只加在同一版本下，所以不升协议版本，`TestWordSourceIsAnAddition` 锁住两个方向的兼容。
- 日语查词要用原形，但现有分词给不了原形和读音：`packages/core/segment/segment.go:174-176` 有意用 `ipa.DictShrink()` 加载了去掉 features 的 IPA 词典，只保留词界和词性，以节省内存，手机也受益于此；`:191` 把助动词并进前一个词（`行った` 是一个词，没有单一的 kagome 原形）；中文的 gse（`:203-210`）只返回字符串，没有读音。因此原形和读音要单独做（P1.2）：
  - 新增 `segment.Annotated(lang, line) []Morph{Text, Word, Base, Reading}`，内部用另一个按需加载的完整 features 词典 `ipa.Dict()`，只在桌面上、并且启用了日语的词典包或词表包时才加载（12.2 第 5 条的测量门槛已达标，数字见第 15 节）。`Words` 保持用 `DictShrink`，`core.WordToken` 不改，手机的分词不受影响。合并后的词（`行った`）取它首个 token 的原形（`行く`），读音是各 token 读音（片假名）连起来（`イッタ`），有一个 token 没有读音就整个留空。Annotated 会等词典读完，所以只在后台 goroutine 里调。`lang` 是 `ja` 时即使整行没有假名也按日语切（`Words` 会把只有汉字的行当中文），是 `zh` 时按中文切，其余按文字判断，和 `Words` 一样。
  - P1.2 先在 Ubuntu 编译机（`ubuild`）上量出 `ipa.Dict()` 相对 `DictShrink()` 的常驻内存增量（RSS）和首次加载时间。门槛是增量不超过 200 MiB、加载不超过 3 s（12.2 第 5 条）。不达标就退到另一条路：Annotated 不出 Base 和 Reading，查词先查表层形，查不到再查拆开后首个 token 的表层形，读音只取自词典包的条目。测量数字和最终选择记在第 15 节：增量约 92 MiB、加载约 0.6 s，达标，用完整词典，没有走退路。
  - 中文的读音（拼音）只来自 CC-CEDICT 这类词典包，gse 不提供读音。
- 验收：装了对应的词典包后，断网、没配 AI 时，在桌面和手机上点日语或中文的词，都能看到释义。

**E2 词表与逐词标注（P1，P1.4 已实现）**
- 接入位置：`apps/kando/film/subtitle.go` 的 `type lineWords struct` 加了 `marks func() []subtitle.Mark`（在 `subtitleLayer` 里为每行原文建一个 `core.NewMemoFunc`，经 `live.Func` 读 `film.App.CueMarks(cue)`，只在这行的标注变了时才让 spans 重算）。`withWords` 改成调 `lineSpans`：在词的边界和每个标注的边界处切开（画线的标注也切，因为标注和词的切分不一定一致），带读音的一段是 `core.Span.Attachment`（一个上面是小一半字号的读音、下面是原文的 view，`VerticalAlignTextBottom`），点它照样查它所在的词；其余标注不改字的颜色（字保持用户选的字幕颜色），而是在字下面画线：有 `Tone` 的画一条该等级颜色的粗实线（`TextDecorationUnderline`，`TextDecorationColor` 取 tone 的颜色，`TextDecorationThickness` 是字号的 9%，`TextUnderlineOffset` 是 14%），`Underline`（已学词）画虚线（`DecorationDashed`），两者都有时是 tone 颜色的虚线，所以等级和已学词永远不会看起来一样，彩色的线在任何字幕颜色上都看得见（owner 的决定）。NSGo 固定版本的 `core.Style` 有下划线的颜色、样式、粗细和位置（`core/style.go:220-230`），macOS、Windows、Linux 的文字后端都画得出来，不需要绕行。描边模式下原文要画十份（撑开大小的一份、九份黑色描边、最上面的一份），读音会改变行的排版，所以下面的九份和撑开大小的那一份也带上同样的读音 view（`withReadings`，不接点击，不画线），没有读音时它们照旧直接画 Content。
- 颜色不写字面量：`accent`、`warn`、`success`、`muted` 分别取 `ui.Colors` 的 `Accent()`、`Warn()`、`Up()`、`Muted()`（播放器那棵子树用深色主题）。
- `Mark` 定义在叶子包 `film/subtitle`（`Start, End int`、`Ruby`、`Tone`、`Underline`，字节偏移落在这一行原文里），所以 `film` 不导入插件包。计算放在非界面包 `plugins/annotate`，它有一个字段完全相同的 `annotate.Mark`，app 用类型转换 `subtitle.Mark(m)` 换过去，两边字段一旦不同就编译不过。一个词可以有两种标注：带 `Ruby` 的只盖住它的汉字部分（`食べた` 的读音 `た` 只在 `食` 上，前后相同的假名去掉），不带 `Ruby` 的在整词下面画线。
- 数据来源：日语的读音来自 P1.2 的 `segment.Annotated`（IPA 词典的片假名读音换成平假名），只在启用了日语词典包、或选了日语词表时才读那份大词典，否则日语没有读音，词表和已学词也只按表层形查；不读大词典时，凡是 `segment.Annotated` 会按日语切的行（说话语言是 `ja`，或者说话语言不是 `zh` 而行里有假名，比如英语片里夹一句日语）都改用 `segment.Words` 和小词典切，免得一部非日语影片因为一行假名就读进那份约 92 MiB 的词典；假名注音去掉前后相同的假名时，长音符 `ー`（含半角 `ｰ`）和半角片假名也算假名（`ラーメン屋` 读 `らーめんや` 只在 `屋` 上标 `や`），半角浊音符号 `ﾞ` `ﾟ` 不合并，遇到时读音盖住的范围宽一些；中文的读音（拼音）取自已启用的中文词典包（`a.pluginsDictsFor("zh", "")` 的顺序，第一个有读音的），同一个词一次计算里只查一次；等级来自选中的 wordlist 包，日语先按原形再按表层形查，只给语言和词表 `language` 相同的行上色；已学词来自生词本（`savedBook().Words` 里说话语言和这部影片相同的词（`annotate.SpokenIn`），按 `wordlist.Key` 小写比较），没有用 study 的 `WordIndex`，因为要一次判断整部影片的所有词。没有词频。
- 何时计算：插件块自己的 `playing` 和 `afterChange` 钩子（都是 `pluginsMarksFollow`；`afterChange` 管暂停时的改动，比如改了一行字幕、往生词本里加了词）每次只算一个可比较的键 `pluginsMarkKey`：视频、字幕哈希（`pluginsSubsHash`：只哈希每行的起止时间和原文（`film.SplitCue`），不含译文，所以翻译一批批写进来、或只改了译文时不重算；播放器换字幕时总是换一个新切片、从不原地改，所以只在切片变了时才重新哈希）、说话语言、三个开关、选中词表的 id 和内容哈希、已打开的中文词典的 id 和哈希、是否读大词典，以及生词本的内容指纹（`pluginsVocabHash`：和影片说话语言相同的词按 `wordlist.Key` 各取 `maphash.String` 哈希（本次运行的种子，不分配内存）后求和，再带上词数，与顺序无关；生词本是原地改的（`study.Book` 的 `slices.Insert`/`Delete`），改它的地方很多（生词本、收藏的句子、两种同步），没有用改动计数器，因为每处都得记得加一，漏一处下划线就不更新，所以每次报进度都扫一遍，只在下划线开着时才算；加词、删词、换词都会重算）。键和已有标注的键、正在算的键都不同时，才在后台 goroutine 里 `annotate.Lines`，算好经 `a.update` 存成不可变的切片；同一时刻只有最新的一份在算：开始新的一份、或键又回到已有标注的键时，前一份的停止标记（`pluginsUI.stop`，`annotate.Sources.Stop`）被置上，它在下一行之前停下，结果即使送回来也丢掉，免得它算完后盖掉当前的标注。它忘掉已读的词表（不再是选中的那份）时也报告有变化，因为插件页的问题区读它。开关在设置里改动时也调一次它；三个开关都关掉或影片没有字幕时丢掉标注。字幕文件被改写后播放器重读（`reloadOpenSubtitles`，字幕还没读完时 `subs.ready` 是 false）期间，它既不丢标注也不丢正在算的那一份，读回来的原文没变就直接沿用。缓存只留当前影片这一份（`pluginsUI.marks`），换回之前看过的影片会重算。不放在 `shelfChanged` 里算，因为每次任务报进度都会触发它；也不去 film 块的打开函数里插调用。绑定里只读缓存，从不调用插件，也不在里面算哈希：`CueMarks` 核对缓存的视频和行数与当前相同，再核对这一行的原文和算标注时的原文（`pluginsMarks.originals`）相同，所以字幕换了切片、新标注还在算的时候，原文没变的行照旧显示旧的标注，读音不会消失再出现、行高不会跳；原文改过的行在新标注到之前不显示标注，旧标注不会画在新文字上。
- 开关在 设置 › 字幕外观 最后的「逐词标注」卡片（见 7.4）。
- 手机端暂时没有标注：手机同步协议没有这些字段，要加得改协议和手机界面，另做。

**E3 术语表包（P1）**
- 接入位置：`apps/kando/state_glossary.go` 的 `glossaryStart` 末尾的 `return job, core.MergeTerms(own, series, global)`，改成 `plugins.AddTerms(core.MergeTerms(own, series, global), a.pluginTerms(job))`。`plugins.AddTerms` 把包的条目接在用户的术语表后面，按 `core.SameTerm` 的规则用 map 去重（去掉首尾空白，每个字母换成它的简单大小写折叠 `unicode.SimpleFold` 里最小的那个，和 `strings.EqualFold` 一致，`ſ` 和 `s` 算同一个词），先到的留下，不复制进 glossary.json 那份切片；不直接把包的条目塞进 `core.MergeTerms`，因为它对每一条都线性扫描已有的结果，一个 5000 条的包每次要比较约 1250 万次。按包声明的 from/to 和任务语言过滤：目标语言取任务的 `RedoLanguage`（在改译成别的语言时）或 `Language`，两者都没有时取设置里的翻译语言（`queueRun` 在找完术语表之后才给这样的任务补上语言），经 `core.TargetTag` 换成标签，认不出的目标语言一条也不加；包的 `to` 和它是同一个标签，或其中一方没有文字和地区子标签、语言相同（`zh` 的包对 `zh-Hans`、`zh-Hant` 都适用，`zh-Hans` 的包不适用于 `zh-Hant`）；听到的语言（`SpokenLanguage`，没有就用用户指定的 `SpeechLanguage`）已知时，还要和包的 `from` 是同一种语言，未知时不按来源语言过滤（只有字幕里出现原文的条目才会随批次发出，`core.TermsIn`）。包的条目是 `ScopeGlobal`、不锁定，来源是新加的 `core.OriginPack`（`packages/core/glossary.go`），只在内存里，从不写进术语文件。
- 包的条目排在用户的全部条目之后、按词去重，所以用户自己的条目和 Locked 条目总是优先。
- `func (a *app) FilmTerms()`（播放时用的 `core.MergeTerms(a.gloss.film, series, global)`）同样经 `plugins.AddTerms` 追加一份。它是「术语」标签的绑定，每次状态变化都会重算，所以必须用 map 合并。播放器的「术语」标签也会列出包的条目，但它们是只读的：说明行写「<范围> · 来自插件，可在 设置 › 插件 关闭」（`glossaryMetaPack`），范围、锁定、删除三个按钮都禁用，`CycleTermScope`、`ToggleTermLock`、`RemoveTerm` 对它们什么也不做（`glossaryTerm` 找不到包的条目）。用户在下面的输入框给同一个词加译法时，存成这部影片自己的条目（不沿用包条目的全局范围），之后它优先。改字幕时顺带改到包的条目（`correctSpreadTerm`），同样存成这部影片自己的、用户锁定的条目，撤销时把用户的条目删掉、让包的条目重新露出来，不会把包的条目写进术语表。
- 「术语」标签只列包里出现在这部影片原文字幕里的条目（`plugins.TermsIn`，判断和 `core.TermsIn` 一样；字幕还没读进来时一条也不列），翻译仍然拿到包的全部条目（`glossaryStart`）。一个 5000 条的包不会在标签里建几千行；`plugins.TermsIn` 先把所有原文小写后拼成一段、对每条做一次子串查找，只有找到的才逐行用 `core.TermIn` 确认，所以不需要在绑定之外缓存。
- 每个包最多 5000 条。

**E4 AI 服务预设（P1）**
- 现在服务商的身份只由包级函数决定，这两个函数只认内置预设：`settings/presets.go:72` 的 `PresetByID(m, id)` 只遍历 `Presets(m)`，`:82-92` 的 `ProviderID(base)` 只遍历 `Presets(locales.In(locales.Source))`，认不出就返回 `"custom"`。所以只在调用方追加插件预设没有用：`settings_ai.go` 的 `syncLLM` 会把插件的地址认成 `custom`，`selectLLM` 会拒绝 `pack.` id，`llmRemember` 会把服务存到 `custom` 下。
- 改法：预设解析要知道当前 app 有哪些预设。`PresetByID(list []Preset, id)` 和 `ProviderID(list []Preset, base)` 都改成接收预设列表，app 提供 `a.llmPresets(m)`，内容是 `settings.Presets(m)` 加上 `a.PluginPresets()`（P0.1 时只有 `settings.Presets(m)`）。`m` 是调用方要的语言：引导页用 `locales.For(cx)`，反馈 issue 用固定的 `feedbackIssueLanguage`，其余用 `a.messages()`，这样各处显示的名称和改之前一样。`feedbackTranslator` 原来是不带 app 的包级函数，改成接收预设列表，P0.1 时由 `feedbackFacts` 传入 `a.llmPresets(locales.In(feedbackIssueLanguage))`，P1.1 起只传内置预设（见本节最后一条）。所有调用处一起改（不能只改一半）。`PresetByID` 的调用处：`settings_ai.go` 的 `savedLLM.clean`、`selectLLM`、`openLLMKeyPage`、`adoptLLMModel`，`bind_onboarding.go` 的 `OnboardingAI`，`bind_settings.go` 的 `CurrentPreset`，`state_feedback.go` 的 `feedbackTranslator`，`state_phonesync_settings.go` 的 `phoneSettings`；`ProviderID` 的调用处：`settings_ai.go` 的 `syncLLM`、`llmRemember`，`bind_settings.go` 的 `ProviderID` 绑定，`state_feedback.go` 的 `feedbackTranslator`，`state_phonesync_settings.go` 的 `phoneSettings`。（按函数名列出，不写行号，免得后面的改动让它们对不上。）`savedLLM.clean` 里没有 app，只传 `settings.Presets(...)`，插件服务靠 P0.2 的 `pack.` 前缀保住。另有三处直接读 `settings.Presets` 的列表，按用途分开处理：`bind_settings.go` 的 `Presets()` 绑定是设置 › AI 下拉框（`settings/ai.go` 里的 `a.Presets()`）的数据源，P1.1 改成 `a.llmPresets(a.messages())`；`bind_onboarding.go` 的 `OnboardingServices`（引导页服务列表）和 `state_feedback.go` 的 `feedbackPublicEndpoint` 只认内置预设，不改。这一步作为 P0.1 单独成 PR，行为不变，涉及约 13 处、6 个文件。
- 插件预设的 id 统一用 `pack.<id>`，只预填 BaseURL、Model、KeyURL、Group。key 和 host 的绑定规则不变（`SameHost`、`restoreLLM`）。
- 必须同时修一处：`settings_ai.go` 的 `savedLLM.clean()` 遇到未知 preset id 时会 `delete(s.LLMServices, id)`，插件暂时缺席（比如所在的盘没挂上）时会把用户的 key 丢掉。改成保留 `pack.` 前缀的服务（前缀是 `settings_ai.go` 的常量 `llmPackPrefix`；没有地址的服务照旧丢掉）。P0.2 时还没有插件预设，它的测试直接在 `pack.example.gw` 下存一个 key、在没有这个预设时跑 clean，断言 key 还在；「先在插件预设在场时让服务真的存到 `pack.<id>` 下，再拿掉插件、跑 clean」的端到端测试随 P1.1 的 ai-service kind 一起加（见第 9 节的测试策略）。
- 防钓鱼：pack 预设的 `baseURL` 和 `keyURL` 只接受 https，只有回环地址（`127.0.0.1`、`::1`、`localhost`）可以用 http。内置服务商的名称加入保留名单（见 6.4）。服务商下拉框里，插件预设排在全部内置预设（含「自定义」）之后，每一行写成「插件 · <包名> · <host>」（`settings.Preset.Plugin` 标出来源包，`settings.Host` 取 host）。NSGo 固定版本的 `ui.Select` 没有带标题的选项分组，所以暂时只靠排在后面和行首的「插件」区分，代码里留 `// TODO(nsgo): …`。点「获取 API Key」时，插件预设先弹一个系统提示框（`base.ShowAlertDialog`），标题写目标 keyURL 的 host，正文写出给出这个地址的包名，用户点「打开」才打开 keyURL；内置预设照旧直接打开。
- 插件预设在启动时晚到：`restoreLLM` → `syncLLM` 在读插件目录之前就按地址认服务商，这时 pack 的地址会被认成 `custom`。所以插件目录读完、以及开关、安装、卸载之后，插件块都会重新认一次服务商（`pluginsFollowLLM`）。规则是不替用户换：当前选的服务只要还在、地址还对得上就保留（`llmProviderFor`），选的是「自定义」就一直是「自定义」，所以 baseURL 和内置预设相同的包不会被换成内置预设，地址恰好和新装的包相同的自定义服务也不会被换成包的预设；只有当前的服务已经不在了，才按地址重认（`settings.ProviderID`）。
- 正在用的包服务消失时（关掉、卸载、换成别的版本、盘没挂上），不能让它的地址和 key 存进 `custom`，否则会盖掉用户自己的自定义服务。做法：开关、卸载、替换和每次读目录之前，先把正在用的包服务按 `pack.<id>` 记下（`pluginsKeepLLM` → `llmRemember`）；消失之后服务显示成「自定义」，但记作 gone（`pluginsUI.gone`，id 和地址），此后 `llmRemember` 和存 settings.json 时插件那块的 `save` 钩子（排在设置那块的 `saveLLM` 之后）都把它算在 `pack.<id>` 名下（`pluginsGoneService`），包回来时它重新显示成包的预设。用户选了别的服务（不是「自定义」）就忘掉 gone（`llmChanged` 钩子）。上次存的服务商若是 `pack.<id>`，启动时就作为 gone 恢复（`restore` 钩子），所以插件目录读完前切换服务也不会写进 `custom`，读完后它显示成包的预设。`llmRemember` 不再直接用 `settings.ProviderID` 的第一个匹配，而是用 `llmProviderFor` 认出当前服务的 id。
- 反馈 issue 的「翻译服务」一行（`feedbackTranslator`）只认内置预设（`settings.Presets`），包服务和其他自定义地址一样写成「自定义」，包名（比如公司内网网关的名字）不会写进公开的 issue。

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
- 改为单独做一个 `pluginSites` 匹配器（实现是 `pluginsUI.sites`，每个包一个 `kcore.VideoMatcher`，在 `state_plugins_sites.go`），只给内置浏览器用：工具栏的「下载视频」（`browseNotVideo`、`browseList`、`browseEnqueue`，`state_browse.go`），以及网页里按钮的两个入口 `browsePageAsk` 和 `browsePageAdd`（`state_browse_buttons.go`，经 `browsePageSite` 判断页面的 origin、`browsePageVideo` 判断链接，二者都用同一个匹配器，不信页面发来的任何东西）。`browserExtVideo`、remote 的 `browser.state`、`browser.add`、`browser.show` 和 `sites` 都不认插件站点；CLI 和 MCP 经 remote 的 `add` 加进来的插件站点链接，按任何未知网站的链接处理，不算站点包的任务。这些都写了测试锁住（`plugins_test.go` 的 `TestPluginSiteWidensNoGate`、`TestPluginSitePageButtons`）。
- 网页里的按钮（视频页上的「下载视频」和缩略图上的 +）覆盖已同意的插件站点：`browse.ScriptWith(sites)` 把这些网站的规则接在看懂自己的网站后面写进页面脚本（每个网站先过 `kcore.ValidateVideoSite`，不过的不写；JSON 由 `encoding/json` 转义 `<`、`>`、`&`、U+2028、U+2029，写不出 `</script>`）。插件网站没有按钮位置，所以「下载视频」浮在播放器角上，标题取文档标题。页面脚本是 `core.WebView.InjectedJavaScriptBeforeContent`，NSGo 只在 web view 创建时读一次（`nsgo/core/webview.go:23-24` 是静态字段，只在 `mountWebView` 的 `:141-142` 读进 config；平台的 `Configure` 其实已经能换脚本，如 `internal/platform/apple/web.go:373-378`），所以已同意的网站一变（安装、卸载、开关、勾选域名），`browseSitesChanged` 就让 `browse.View` 用 `core.Dynamic` 重建 web view，载入它原来显示的页面，但之前的历史记录没了。已提交 [nsgo-dev/nsgo#840](https://github.com/nsgo-dev/nsgo/issues/840)，绕行处写了 `TODO(nsgo#840)`。站点包的清单没有按钮位置（`rows`、`titles`、`cut`）和扩展菜单（`menu`）这些字段，写了就是未知键。
- 在站点包的、已同意的域名上，「下载视频」按包的链接规则判断：匹配的是一个视频，可以加；该域名下的其他页面一律不是视频（按钮变灰，提示和别的列表页一样），也不当成播放列表或频道去列视频（`browseList` 返回 false），所以不会经 `playlistAsk` 用浏览器登录去读列表。没同意的域名、停用的包，和没装包时一样。
- 站点包加的下载（工具栏和网页按钮都一样）一律不带 `--cookies-from-browser`，也不带内置浏览器页面的 cookie。接点：`browseEnqueue` 加入时经 `jobQueue.stamp` 给任务打上 `core.Job.PluginSite`（包 id，存进 library.json）并把 `Cookies` 设为 `core.CookiesNone`；`queueRun` 不给这类任务补设置里的浏览器；`queuePipeline` 只要 `PluginSite` 非空就把 `Tools.Cookies` 和 `Tools.SiteCookies` 都清空，不管 `Cookies` 被谁改过（失败卡片的「借用登录」也一样）。所有用 yt-dlp 的地方（下载、重做、仅字幕影片的串流 `StreamURL`）都经 `queuePipeline` 取 Tools。失败卡片（`FailureRemedy`）对这类任务去掉借用登录、查找浏览器、选择浏览器和完全磁盘访问权限的按钮，说明改成「插件添加的网站上的视频，下载时一律不借用浏览器登录」。
- 前置条件（P4.0）：抽出共用的 `core.ValidateVideoSite` 和 `ValidateVideoSites`，只允许 Go 和 JS 共有的正则子集，不允许嵌套量词、反向引用和环视，长度不超过 256，域名唯一。站点包和 `pluginSites` 用它校验。`catalog.Load` 和 `compiledVideoSites` 不再需要改。实现时比这几条更严，见第 15 节 P4.0。
- 每个域名都要用户单独同意：安装确认框和包的详情里每个域名一个复选框，默认不勾；同意的域名存在 state.json 里这个包的条目上（`plugins.PluginState.Sites`），和 `enabled` 并列，旧文件没有这个字段照样能读，读出来是一个都没同意。不接受 sign-in cookie 配方，不新增 `core.Platform`。
- 站点包的清单：`site.sites` 是 1 到 8 个网站，每个有 `domain`、可选的 `not`（域名下不是视频页的主机，最多 16 个）和 `videos`（1 到 16 个链接规则：`path`，以及成对出现的 `param`、`value`，和 `kcore.VideoShape` 同义）。只有数据：没有脚本、cookie、登录配方、yt-dlp 参数，也没有按钮位置。域名必须是可注册域名或它下面的主机，不能是公共后缀（`co.uk`、`github.io`，用 `golang.org/x/net/publicsuffix` 判断，`apps/kando` 已经依赖 x/net），也不能等于、在下面或在上面包含看懂自己的 `kcore.VideoSites` 的域名（`ErrSiteKnown`）。

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

kind 是封闭枚举：`dictionary`、`wordlist`、`glossary`、`ai-service`、`translate-style`、`task`、`export-template`、`site`、`script`。遇到不认识的 kind，在设置页的问题区列出来，不加载。本轮（P0、P1）只实现 `glossary`、`ai-service`、`dictionary`、`wordlist` 四种，P4.1 起加上 `site`；其余几种解析器认得，但 `Validate` 用哨兵错误 `ErrUnsupportedKind` 拒收，问题区显示「这个版本的看懂还不支持这类插件」，到各自那一期再放开（12.2 第 3 条）。

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

解码分两步：先只读 `schema`、`id`、`kind` 三个头部字段；`schema` 不是本版读的那个（数据包的 1）时先返回 `ErrSchema`，不再看 kind 和其余键，因为新版格式里可能有本版不认识的 kind 和键；kind 不在枚举里时返回 `ErrUnknownKind`；kind 在枚举里但本版还没实现时，返回只带头部的 Manifest，由 `Validate` 以 `ErrUnsupportedKind` 拒收，不对其余字段做严格解码（那些字段的类型要到对应的一期才定义）；kind 已实现时，再用 `DisallowUnknownFields` 严格解码全文。

通用规则：

- **id 在所有 kind 之间唯一**：state.json 和 `Remove` 只按 id 认包，所以一个 id 只能是一个包。`Swap` 遇到别的 kind 下已经装了同一个 id 时返回 `ErrIDTaken`，什么都不改，要先卸载那个包；手工拷进插件目录造成的重复，`Scan` 按 kind 的读取顺序保留第一个，后面的报成 `ErrIDTaken` 的 Issue。
- **id 的格式是 `<publisher>.<name>`**，必须匹配 `^[a-z0-9]+(?:[.-][a-z0-9]+)*$`，至少含一个 `.`，不超过 64 字节，以点分开的每一段都不能是 Windows 的设备名（`con`、`nul`、`com1` 之类，所有平台上都查）。这个正则排除了 `.`、`..`、`a..b`、以点或连字符开头和结尾的写法。第一段是发布者：`kando.` 保留给官方包；签名脚本包的发布者段必须和证书覆盖的 app-ID 模式一致（`ai.yldm.kando.plugin.<publisher>.*`），由 `plugins.Validate` 检查。未签名的数据包也用这个格式，但发布者段未经验证，来源徽标一律是「本地」，而且不能用 `kando.`。
- **路径安全**：所有按 id 拼出来的路径（`root/<kind>/<id>`、staging、Remove）在拼完之后都要过两道检查：`filepath.IsLocal(id)`，以及 `filepath.Rel(root/<kind>, 结果)` 不以 `..` 开头，确认结果确实位于 `root/<kind>/` 下面。
- `name`、`description` 以及所有 label 都是多语言 map，`en` 必填。
- `schema` 只在旧版 Kando 必须拒收时才升：数据包为 1，script 为 2，这样不支持脚本的旧版和手机会在版本检查时就拒绝。新功能通过新增键表达，旧版遇到不认识的键会整包拒收，自然安全地拒绝。
- **数据包的包形状**：平铺目录，最多 16 个文件，manifest 不超过 256 KiB，zip 文件不超过 32 MiB（dictionary 放宽到 200 MiB、64 个条目）。解压后的总字节数也设上限：数据包 64 MiB，dictionary 1 GiB。计数按实际读出的字节累加，不信任 `UncompressedSize64`；dictionary 里嵌套的 yomitan-zip 共用同一个计数器和条目上限，嵌套只允许一层；计入的是嵌套 zip 解压出的字节，zip 文件本身为算哈希读出的字节不重复计入，只受一个同样大小的独立上限约束。不允许符号链接，文件名只能是 ASCII，拦截 zip-slip 和绝对路径。每个文件都必须在 manifest 里被引用，`.md`、`.txt` 说明文件除外。
  - 文件名只能由 ASCII 字母、数字和 `.`、`-`、`_` 组成，不以 `.` 开头或结尾，不超过 100 字节，也不能是 Windows 的设备名（`con`、`nul`、`com1` 之类，看第一个点之前的部分）。最后一条在所有平台上都查，同一个包在三个平台上的结果一致。
  - zip 里的 manifest 必须在顶层，不能有目录条目，所以「把整个文件夹压缩」得到的 `<文件夹>/kando-plugin.json` 会被拒收。zip 里两个名字只差大小写也拒收，因为在不分大小写的盘上它们是同一个文件。
  - yomitan-zip 内部只检查不解压：条目名可以带子路径，但不能有 `..`、绝对路径或反斜杠，不能是符号链接，不能再套 zip。条目数（目录条目也算）和 manifest、外层文件共用 64 的上限，P1.3 接真实词典时如果发现大型 Yomitan 词典超出，再调这个上限。里面的文件校验和不符或解压出错，算 `ErrBadArchive`。
  - `archive/zip` 打开 zip 时会先把中央目录的每一条记录都读进内存，不管记录数是多少，所以一个由几百万条空记录组成的 zip 会在任何上限生效之前就分配几 GB。外层 zip 和嵌套 zip 都经 `openZip` 打开：它在 `zip.NewReader` 读目录期间最多放行 1 MiB 的读取，超出就是 `ErrTooLarge`；64 个条目的目录加上最长 64 KiB 的注释远小于这个数。
- **从文件夹安装**：用 `Lstat` 逐项检查，遇到符号链接或特殊文件就拒绝，再把普通文件复制进 staging，最后校验的是 staging 里的副本。不能先校验原目录再复制，那样有 TOCTOU 问题。打开每个文件后再用 `os.SameFile` 比对 Lstat 的结果，检查和打开之间被换成链接的文件也会被拒绝。文件管理器写进文件夹的文件（访达的 `.DS_Store`，资源管理器的 `Thumbs.db`、`desktop.ini`，后两个不分大小写）在复制时跳过，`Validate` 列文件时也跳过：不检查、不计数、不算进哈希，也从不读取。所以用户在访达里打开已安装包的目录后，下次 `Scan` 这个包照样有效、哈希不变。zip 里的这些文件仍按普通文件名规则处理（`.DS_Store` 以点开头，被拒收）。
- **内容哈希**：对排序后的「路径、长度、字节」序列做 SHA-256，授权绑定在这个哈希上。路径和内容的长度都写成 8 字节大端整数，字节在名字和内容之间、或在相邻文件之间挪动都会改变哈希。
- **字段规则**（`plugins.Validate`）：`version` 由字母、数字和 `.`、`+`、`-` 组成，不超过 32 字节；`license` 必填，不超过 64 个字符；`author` 可选，不超过 80 个字符；`homepage` 可选，规则同 https 地址；`Text` 的键必须是规范写法的语言标签（`zh-Hans`，不是 `zh-hans`），这样 `Text.In` 能用 app 的语言直接找到；名称和 label 不超过 40 个字符，`description` 和 `attribution` 不超过 500 个字符，都先清洗再计长度。glossary 的 `from`、`to` 是语言标签，1 到 5000 条，每条原文和译文各不超过 200 个字符；ai-service 的 `group` 只能是 `cloud` 或 `local`，`model` 不超过 200 个字符，地址不能带用户名和密码；dictionary 的 `format` 是 `yomitan-zip`、`cc-cedict` 或 `tsv`，只有 yomitan-zip 的文件以 `.zip` 结尾（yomitan-zip 是 Yomitan 导出的词典 zip，只读其中的 `term_bank_<n>.json`；cc-cedict 是 CC-CEDICT 的 UTF-8 文本；tsv 每行是 `词条<TAB>读音<TAB>释义` 或 `词条<TAB>释义`，`#` 开头的行是注释，第三列之后的列忽略）；wordlist 有 1 到 32 个 tier，tier 的 id 匹配 `^[a-z0-9][a-z0-9-]{0,31}$` 且不重复，`tone` 是 `accent`、`warn`、`success`、`muted` 之一；它的文件每行是 `词<TAB>tier 的 id`，之后的列忽略，`#` 开头的行是注释，没有 tab 的行和 tier 没声明过的行忽略，同一个词再出现时保留第一次的 tier，词按小写比较，一个词也没留下的词表打开时以 `wordlist.ErrEmpty` 拒绝（P1.4）。清单里只能有自己那个 kind 的字段。

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

数据包里不能出现 key、cookie、yt-dlp 参数或任何可执行内容。ai-service 的 `baseURL` 和 `keyURL` 必须是 https，只有回环地址可以用 http。包里的地址（还有 `homepage`）都必须有主机名，不能含 Bidi 控制符、零宽字符等看不见的字符，因为打开前要把地址给用户看。

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
func Swap(root, staged string, p *Pack) error // old → .old-<rand>, staged → root/<kind>/<id>, p.Dir = the new folder; ErrIDTaken when another kind has the id; caller closes open handles first
func Remove(root, id string) error           // id checked with IsLocal + Rel before any path is built
func DefaultRoot() string                    // <UserConfigDir>/Kando/plugins

var ReservedNames []string // see 6.4

// State is plugins/state.json: single writer, tmp+rename, never synced, never in settings.json.
type State struct {
	Format   int           `json:"format"`
	Revision int           `json:"revision"` // one higher on each Save
	Plugins  []PluginState `json:"plugins"`
}
func LoadState(root string) (State, error) // a newer (ErrStateFormat) or unreadable file, or one without a format (ErrBadState): every pack off
func (s *State) Save(root string) error    // refuses to write over a newer format; after a bad file it writes only the entries s has
func (s State) Enabled(id string) bool     // the entry's Enabled; a pack with no entry is on (6.2)
type PluginState struct {
	ID      string `json:"id"`
	Hash    string `json:"hash"`
	Enabled bool   `json:"enabled"`
}
```

P1 的数据包没有授权：glossary、ai-service、dictionary、wordlist 装好就启用（6.2 节），`Hash` 只用来认出「同一个 id 换了内容」，好在列表里标出「已更新」。

state.json 读不了、不是 JSON 或没有 format（format 为 0，比如被截成 `{}` 或手工改过）时是 `ErrBadState`，format 比本版新时是 `ErrStateFormat`，两种情况下所有包都按停用处理。这个「全部停用」只维持到下一次 `Save`：`Save` 只写 `Plugins` 里已有的条目，写完后没有条目的包又按启用处理。所以 P1.1 的 app 在发现 state.json 损坏后第一次保存之前，要先给 `Scan` 到的每个包补一条 `enabled: false`，否则用户拨一次开关，损坏前关掉的包就全被重新打开了。格式更新的文件 `Save` 不覆盖；`Save` 写之前会重读磁盘上的 state.json，读到之后被更新版本的看懂改写过的也返回 `ErrStateFormat`，revision 取自己和磁盘上两者较大的再加一。没有 format 的文件按损坏处理，会被覆盖。

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
// in apps/kando/plugins/dict (P1.3 as implemented): a concrete type, not an interface, as it has one implementation until P3's script lookUp
type Dict struct{ /* the open index file */ }
func Open(p plugins.Pack, cache string) (*Dict, error) // builds <cache>/<id>_<hash16>.idx the first time this content is opened; removes no other index
func Forget(cache, id, keep string)                    // removes the pack's indexes but keep; the app calls it on the engine goroutine's side after each Open, keeping the content installed then
func (d *Dict) ID() string
func (d *Dict) LookUp(word, base, reading string) (lookup.Entry, bool, error) // base first, then word; reading picks among homographs only for a hit on word itself
func (d *Dict) Close() error

// in apps/kando/film/subtitle (leaf package), so film does not import plugins (P1.4 as implemented)
type Mark struct {
	Start, End int    // byte offsets in the line's original
	Ruby       string // a reading over these characters; a mark without one has Tone and Underline
	Tone       string // accent|warn|success|muted, mapped by the host to theme roles
	Underline  bool   // a word in the vocabulary
}

// in apps/kando/plugins/annotate: the same Mark, which the app converts with subtitle.Mark(m)
func Lines(lines []string, s Sources) [][]Mark // off the UI thread: may wait for segment.Annotated's dictionary

// in apps/kando/plugins/wordlist
func Open(p plugins.Pack) (*List, error)
func (l *List) Tier(word string) (plugins.Tier, bool)
```

替换同 id 包的顺序：先在引擎 goroutine 上关掉这个包的词典句柄并 Close 它的脚本，然后把旧目录重命名为 `.old-<rand>`，再把 staging 重命名为目标目录，最后重新打开。对已存在的非空目录直接 rename 在任何平台上都会失败，所以必须先挪走旧目录。P1.3 的词典索引不放在包目录里，而是放在插件目录的 `.index/` 下（`<id>_<哈希前 16 位>.idx`），长期打开的只有这个索引文件，包里的文件只在建索引时读一遍，所以 `Swap` 换包目录时没有打开的句柄，原先设想的 Windows 共享冲突重试和待替换标记都不需要，没有做。剩下的情形是建索引的那几秒里替换同一个包：Windows 上 rename 会失败，安装报错，用户重试即可。旧目录换下来后 `Swap` 立刻删掉它，删不掉（比如文件还被占用）就留给下一次 `Stage` 按 24 小时规则清理，安装照样算成功。

### 5.2 app 一侧（package main）

```go
// state_plugins.go (P1.1 as implemented; the type is pluginsUI so main.go's app struct keeps its alignment and gains one line)
type pluginsUI struct {
	root     string          // the plugins folder; tests set it, else <dir of settings.json>/plugins
	packs    []plugins.Pack
	issues   []plugins.Issue
	state    plugins.State
	stateErr error           // LoadState's: ErrBadState or ErrStateFormat keep every pack off
	scanned  bool
	scanGen  int             // only the last read's answer is kept
	order    []string        // savedPlugins.PluginOrder
	busy     bool            // an install, removal, switch or staging runs
	err      error           // why the last one failed, mapped to catalog text in bind_plugins.go
	pending  *pluginsPending // install confirmation: staged pack, or why it cannot be installed
}
// P1.3 added dicts map[string]*pluginsDict (by id: the content hash, the open *dict.Dict or why it could not be opened; lookup order comes from order) and opening map[string]bool (index names of the contents being opened, so turning a pack off and on during its index build does not start a second build); P1.4 added prefs (the three switches), marks (one film's: its key, the cues they were computed from and the marks per line, immutable once set), marking (the key being computed), subs (the cues last hashed and their hash) and list (the word list read, by id and content hash, or why it could not be read).
// P3 adds health, scripts (engine goroutine only), hosts (one per plugin, never shared) and loading (ids still to Load, one per update tick).

// savedPlugins holds preferences only, never trust.
type savedPlugins struct {
	PluginOrder      []string `json:"pluginOrder,omitempty"` // every pack id seen, in the order it came: list order, glossary precedence; P1.3 lets the dictionary group be reordered
	PluginReadings   bool     `json:"pluginReadings,omitempty"`   // P1.4: readings over words
	PluginWordlist   string   `json:"pluginWordlist,omitempty"`   // P1.4: the word list whose tiers color the words, kept while it is missing
	PluginKnownWords bool     `json:"pluginKnownWords,omitempty"` // P1.4: underline the words in the vocabulary
}
// clean() trims, dedupes, caps at 1000 and keeps ids that are missing now; the page lists those as missing until the user removes them. P1.4 added the three switches as fields of their own (all off by default) and trims the word list's id.

func (a *app) pluginsHooks() areaHooks
func (a *app) pluginKeys(cx *core.Scope) []shortcut // P3
func (a *app) pluginsOverlay(cx *core.Scope) core.Node
func (a *app) pluginTerms(job core.Job) []core.Term
func (a *app) pluginPresets(m locales.Messages) []settings.Preset // names in m.Language()
func (a *app) llmPresets(m locales.Messages) []settings.Preset    // settings.Presets(m) + pluginPresets(m), for PresetByID/ProviderID
func (a *app) vocabResolve(q lookup.Query, film string) vocabAsk // P1.3, engine goroutine: the cache, the open dictionaries, the AI service
func (k vocabAsk) answer(ctx context.Context) (lookup.Entry, string, error) // P1.3, any goroutine: the cache's entry, else lookup.Ask.Answer; the string is the dictionary pack's id, "" for the AI service
```

`pluginsHooks` 各回调：

- `restore`：读 savedPlugins；上次存的 AI 服务商是 `pack.<id>` 时把它记作 gone（P1.1，见 E4）。
- `save`：写 savedPlugins。
- `ready`（P1）：在后台 goroutine 里读 state.json、Scan，经 `a.update` 交回引擎，再在后台打开已启用的词典（P1.3，`pluginsSyncDicts`，开关之后也一样），然后看一眼 CLI 的 inbox。
- `becameActive`（P1.1）：app 回到前台时再看一眼 inbox，所以在终端里跑完 `kando plugin install` 再切回看懂，就会弹出确认。不在这里重新 Scan，免得 P1.3 的大词典每次切回都被重新哈希。
- `ready`（P3 起追加）：在后台 goroutine 里读 health.json。如果发现上次留下的 in-flight 标记，就把对应的 (id, hash) 记为 `hung` 并停用它。然后 Scan、Verify，把结果交回引擎，再按每个 `a.update` 只 Load 一个插件的节奏逐个载入。每载入一个都会触发 `Changed`，菜单和面板经 `areasFollow` → `menuFollow` 补上插件行，不靠 hook 的先后顺序。
- `playing`（P1.4，`pluginsMarksFollow`）：当前影片的键（`pluginsMarkKey`，见 E2）和已有标注、正在算的标注的键都不同时，起后台计算标注（P3 起同时推送 `film.opened` 事件）；其余时候只比较键、什么也不做，返回 false；标注都关掉时丢掉已有的，返回 true。`afterChange`（P1.4）也是它，接住影片暂停时的改动；字幕正在重读时什么也不做。
- `shelfChanged`（P3）：只在任务完成时推 `job.completed`，不计算标注。
- `llmChanged`（P1.1）：用户选了「自定义」和 gone 以外的服务时忘掉 gone（E4）。P3 起再推 `llm.changed`。
- `afterChange`：返回 false。
- `quitStarting`：关掉词典句柄（P1.3）；P3 起还要 Close 所有插件。这时引擎还在跑，`a.cx` 还在。不用 `quitting`，因为那时引擎已经停了，app 的 scope 也没了，而 `Plugin.Close` 要释放的正是那个 scope 的子 scope。

在 `events.go` 的 `areas()` 里，它放在 `settingsHooks` 之后。P1.1 还没有快捷键，所以 `keys.go` 不加 `pluginKeys`，到 P3 再加。

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

- CLI 在 `cli/cli.go` 的命令表里加一行 `plugin`，子命令有 `list`、`validate <file-or-folder>`、`install <file>`，都带 `--json`。
  - `install` 先用和 `validate` 一样的检查验过文件，再用 `plugins.Drop` 复制进 `plugins/inbox/`（先写隐藏文件再改名，文件名带随机前缀），用户在 app 里确认后才会安装（P1.1 已实现）。只接受文件，文件夹报用法错误，文件夹可以拖到 设置 › 插件 页上安装。app 在 `ready` 和 `becameActive` 时用 `plugins.Inbox` 取最早的一个文件，Stage 之后、弹出确认时把它移出 inbox（装不了的也只提示一次）；确认框开着或别的安装在跑时先不取，关掉后接着取下一个。看 inbox 用自己的 `inboxBusy`，不占 `busy`，所以切回前台时插件页的开关和按钮照常可用；看的过程中用户开始了别的安装或开关，inbox 让路，文件留到下一次；开关存完 state.json 后会马上再看一次（`pluginsSetEnabled`），安装和卸载之后的重新读目录也会看。
  - `validate` 离线运行，不连 app：把包 `plugins.Stage` 进一个临时目录（和安装走同一条解压、复制和 `plugins.Validate`），打印 id、kind、版本、英文名和内容哈希，然后删掉临时目录。（P0.3 已实现）
  - `list` 也不连 app：直接 `plugins.Scan` 插件目录，按 state.json 标出开关，并在 stderr 列出不是有效包的目录。（P0.3 已实现）
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
- 来源徽标（官方、已验证、开发者、本地数据包）由证书链推出，不读清单。插件名去掉 Bidi 控制符和零宽字符，最长 40 个字符。保留名单是「Kando」「看懂」「Official」「官方」「公式」「공식」，再加上所有内置 AI 服务商的名称（取自 `settings.Presets` 的显示名），比较时忽略大小写、全半角和空白。`plugins` 包不能导入界面包，所以名单以静态列表 `plugins.ReservedNames` 存在包里（含五种界面语言下的服务商名称），根目录的 `plugins_test.go` 断言 `settings.Presets` 在每种界面语言下的每个名称和 `appName` 都在名单里，内置服务商改名或新增时测试会失败。
- **开发者模式**是设置里的显式开关。dev root 的生成和签名都在 app 里完成，位置是设置 › 插件 › 开发者：「生成开发者密钥」把私钥和公钥都存进 `core.Secrets`；「打包并安装文件夹…」由用户在系统文件面板里选一个源码文件夹，app 用 dev key 签名，然后走和正式包同一条 Stage → Verify → 比对 kandoManifest → 确认 sheet 的路径。不允许用 `tsx.Sandboxed` 直接跑源码，CLI 和 socket 都不能签。开发者包一直带着徽标，不参与更新，不能分享安装，也不允许占用已安装的非开发者包的 id。
- 数据包可以不签名。将来如果支持签名数据包，site 包的高风险能力只对官方和已验证来源开放。

### 6.5 安装与更新

- 安装入口只有三个：设置 › 插件页的「从文件安装…」、把文件或文件夹拖到 设置 › 插件 页上（`desktop.DroppedPaths`）、CLI inbox。三者最后都要经过宿主画的确认框。开发者包另有 6.4 节的入口，同样要经过确认框。拖进主窗口别处的文件由片库那块的 `dropFiles` 接收，要在那里分出插件包就得往别的块的函数里插调用（AGENTS.md 不允许），所以主窗口的拖放安装没有做，只有插件页本身接拖放。一次拖进多个文件时只问第一个。
- 安装流程：
  1. `plugins.Stage`：复制或解压到 `plugins/.staging-<rand>`。zip 按实际字节计数（嵌套 zip 共用计数器），拦截 zip-slip、符号链接和绝对路径；文件夹用 Lstat 逐项复制。
  2. 对 staging 里的副本跑 `plugins.Validate`，包括 id 正则和路径检查。脚本包还要 Verify，并在预读沙箱里比对 `kandoManifest()`。
  3. 静态检查：声明的 host 权限都在已知 handler 表里；签名 Capabilities 里除了 Host 和 Network 没有别的字段。
  4. 用户在确认 sheet 上点「安装」后，在引擎 goroutine 上关掉同 id 包的句柄，再 `plugins.Swap`（5.1 节的顺序）。
  5. 残留的 `.staging-`、`.old-` 目录超过 24 小时才清理。清理由每次 `Stage` 开始时顺带做，不另设入口。rename 会保留目录原来的修改时间，很久以前装的包刚被挪开时看起来也已经过了 24 小时，所以挪开的目录命名为 `.old-<挪开时的秒数>-<随机串>`，`.old-` 目录的年龄从名字里的时间算起，免得另一个 Kando 的 `Stage` 删掉 `Swap` 回滚还要用的旧包。
- 每次启动 Scan 时，用同一个校验器再校验一遍。清单、字段和目录形状每次都查；内容哈希（和词典嵌套 zip 的检查）按包里每个文件的（文件名、大小、修改时间）缓存在插件目录的 `.hashes.json`（`plugins/hashcache.go`，写临时文件再 rename），都没变就沿用上次的哈希，不再把可能解开有 1 GiB 的词典读一遍，`kando plugin list` 也一样。修改时间离这次 Scan 不到 3 秒的文件不缓存（文件系统的时间粒度可能只到秒），下次再读；缓存的格式号变了、读不了或超过 4 MiB 就全部重算，校验规则一旦改到文件内容（哈希、上限、嵌套 zip 的检查）就要升 `hashCacheFormat`。同用户的程序可以改了文件再把大小和时间改回去，让旧哈希继续有效；它本来就能改 state.json，所以这不扩大 6.7 的风险。`Stage` 和 `kando plugin validate` 仍完整校验。
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

- 设置里新增一节「插件」：`settings/sectionid` 加 `Plugins`；`settings/sections.go` 的 `var sections` 加一行（不需要改成函数，插件页一直显示）；页面写在 `settings/plugins.go`，用 `settings/form` 的 Card、Label、Badge、Button 和 `ui.Switch`、`ui.IconButton` 拼。它读自己的接口 `settings.PluginsApp`，不往 `settings.App` 里加方法，照 `Window.Account` 的先例由 `settings.Window.Plugins` 注入（`state_settings.go` 的 `settingsWindow` 里多一个字段）。
- 列表按 kind 分组。每行有名称、来源徽标、版本·作者、行内开关，以及展开详情的按钮（P1.1 没有每行的图标）。P1.1 的状态只有开关本身和「未安装」：savedPlugins 引用了、目录里却没有的包单独列在「不在插件文件夹里」一组（包已经读不到，不知道它的 kind），带「移除」按钮。其余状态到有对应机制的那一期再加：需要授权 / 已更新，需要重新授权 / 已停用：出错 / 已停用：太慢 / 已停用：上次让看懂停止响应 / 已被撤回 / 离线时不可用。
- 词典分组里每行有上移、下移两个按钮（`lucide.ArrowUp`/`ArrowDown`，在第一行、最后一行时禁用），决定查词时谁先答，顺序存在 `savedPlugins.PluginOrder`。NSGo 固定版本没有可拖动排序的列表控件，所以不做拖动。移动只改顺序，行按包 id 保留并移动（`core.Each`），不重建，按下的按钮保住键盘焦点，可以连按几次。
- 顶部按钮：「从文件安装…」（系统文件面板，配合 `holdModal()`，接受 `.zip` 和 `.kandopack`），以及「在访达中显示」（按平台换成文件资源管理器、文件管理器；`desktop.Reveal`，经 `revealInFolder`，插件目录不存在时先建出来）。开发者模式开着时，多一组「开发者」：生成开发者密钥、打包并安装文件夹…。
- 详情在行下面展开：说明、它会加什么（术语条数和语言、AI 服务的 host 等，由 app 用界面语言写好）、许可、主页、词典的 attribution、id 和哈希前 12 位，以及「卸载…」。卸载的确认也在详情里展开（和手机同步「撤销…」的做法一样），确认后删除目录并从 state.json 移除。state.json 是更新版本写的时，和安装一样在删之前就拒绝，插件页另写一句「没有卸载」（`plugins.ErrUninstallRefused`，和列表上方说明 state.json 的那句不重复）；目录已经删掉、之后 state.json 没存上的，算卸载好了，插件页另写一句「已完成，但没能保存哪些插件是启用的」（`plugins.StateUnsaved`，安装后没存上也是这一句；原因是插件页认得的错误时用它自己的文案），不说卸载失败。
- state.json 读不了或是更新版本写的，在列表上方用一行红字说明所有包暂时停用。
- 问题区列出没能载入的包，以及原因的本地化文案。

### 7.2 安装确认（`settings.PluginInstallDialog`，`ui.Dialog`）

P1.1 的确认框只有数据包要显示的部分：名称（标题）、说明、它会加什么（AI 服务包写明「会把你的 API Key 和要翻译的字幕发送到 <host>」）、来源（「来自这台电脑上的文件 · 未签名」）、版本·作者、许可、id 和哈希前 12 位，以及要替换的已安装版本。取消是默认按钮，回车和 Esc 都不安装。点了「安装」之后、安装结束之前，取消按钮变灰，回车和 Esc 也不起作用：这时 `plugins.Swap` 正把暂存副本移到位，取消会把它删掉，留下一个用户取消了却装上的包，或者一个缺文件的包顶替了旧版本。文件装不了时，同一个对话框只说原因（catalog 文案）和「好」；点了「安装」之后才失败的（另一种 kind 已有这个 id、写盘失败），确认框也不关，换成同样的失败视图；state.json 是更新版本写的时，在 `plugins.Swap` 之前就拒绝，什么都不替换。包已经放到位、之后 state.json 没存上的，算装好了：确认框照常关掉，存不上的原因单独显示在插件页上（`plugins.err`，「已完成，但没能保存哪些插件是启用的：<原因>」）。失败视图等用户点「好」，因为它可能画在主窗口上、插件页根本不在眼前。确认框开着时占用 `holdModal`。它的文字在建的时候取，所以界面语言切换时整个重建（读 `i18n.Language(cx)`，和 `settings.Window` 一样）。它画在哪里：设置在自己的窗口里、并且正停在插件页时，由插件页画；其余情况（CLI inbox、设置画在主窗口上、设置开在别的页）由主窗口的 `pluginsOverlay` 画（`PluginInstallHere` 决定），免得确认框被设置窗口挡在主窗口里。下面几条是脚本插件（P3）要加的部分。


- 显示名称、来源（例如「由 acme 发布，看懂已验证」，取自证书）、版本、大小、哈希前 12 位。
- 「它想要」的清单：每个权限的说明来自 catalog，插件不能自定义权限说明。例如「读取正在播放的影片到当前句为止的字幕」「用你的 AI 服务回答，每天最多 5 万 token」「向 api.deepl.com 发送 POST /v2/translate」。必需权限在上，可选权限在下，默认不勾。
- 「读你的字幕或用你的 AI」与「联网」同时出现时，单独一行标为高风险。凭据一行写明它会发往哪些主机；主机不在内置服务表里时额外警示。
- 开发者来源加一条警示。site 包每个域名一行复选框，默认不勾；重装同 id 的新版本时，旧版本已经同意、新版本仍然列出的域名预先勾上（P4.1）。
- 点「安装」后，脚本插件仍处于停用状态，回到列表打开开关才算授权。

### 7.3 详情页

- 说明和出处（词典必须显示 attribution）。
- 权限按「读取你的内容 / 改动你的内容 / 联网 / 用你的 AI」分组，每一项都能单独撤回。
- 每日 token 用量和预算。
- 插件声明的设置，类型有 bool、choice、number、text、secret，最多 16 项，由宿主用 `settings/form` 加 `ui.Switch`、`ui.Select`、`ui.Input` 渲染。secret 字段的输入框上写着「此密钥只会发送到：<hosts>」。值存在 `plugins/<id>/settings.json`（secret 除外，它在 `pluginhost.` 分区里），不计入哈希，改设置不需要重新授权。
- 按键冲突提示、「查看审计记录」、「卸载」。卸载会删除目录、`plugin.<id>.*` 和 `pluginhost.<id>.*` 两个分区，并从 state.json 移除，删除前要确认。AI 预设插件留下的服务记录会保留，key 不丢。

### 7.4 角色选择的位置

- 设置 › 字幕外观 最后的「逐词标注」卡片（`settings.Marks`，读自己的 `settings.MarksApp`，照 `Window.Plugins` 的先例由 `settings.Window.Marks` 注入，不扩大 `settings.App`）：「注音或拼音」开关（说明写明日语假名要启用日语词典包、或在「按等级画线」里选日语词表，只装了日语词表包而没选它不会读大词典）、「按等级画线」下拉框（「关闭」加上已启用的词表包，名称取包的 `name`；说明写明会在词下面画一条该等级颜色的粗线）、「标出已学词」开关（说明写明是虚线）。三项默认都关。（看懂的 设置 › 通用 › 外观 只管明暗，所以放在字幕外观。）
- 查词卡在底部标出来源，例如「JMdict」或「AI」。学日语或中文的用户第一次点词时，卡片底部提示「装离线词典，查词更快、不耗 AI」，点了打开设置 › 插件。条件是这个词是日语或中文（`lookup.WordLanguage`），插件目录已经读过，并且没有装这种语言的词典包（开没开都算装了）；只提示一次，提示过就记在 settings.json（`savedPlugins.PluginDictHintShown`，`pluginsDictHint`）。
- 设置 › AI 的服务商下拉框里，插件预设排在内置预设之后，每项写成「插件 · <包名> · <host>」（`ui.Select` 还没有带标题的分组，见 E4）。点「获取 API Key」时先用系统提示框显示目标域名再打开。
- 导出字幕的格式下拉框、学习导出的目标列表里出现插件项，后面标出插件名。
- 插件命令出现在命令面板和菜单栏。在 macOS 上，不认识的分组会由现有逻辑放在「窗口」菜单前面，单独成一个菜单（`menu.go:20`）；在 Linux 和 Windows 上，命令面板是唯一入口。

### 7.5 错误

- 插件失败不弹窗打断，在出错的地方说清楚，比如查词卡上写「JMdict 没有这个词，问了 AI」：先问过的词典（`lookup.Ask.Dicts`）都没有这个词、由 AI 服务回答时（查词出错的词典，比如索引坏了、经 `Failed` 报告的，不算「没有这个词」，不列进去），来源一行写「词典「<名称>」没有这个词，问了 AI 服务」，问过几本时写「你的词典都没有这个词，问了 AI 服务」（`vocabSourceText`）。缓存里的 AI 回答没有问过词典，照旧写「由 AI 服务解释」。手机的查词卡不写这一句（`psync.WordSource` 没有这一项）。
- 错误码在 `plugins` 包里是英文哨兵错误（`ErrUnknownKind`、`ErrUnsupportedKind`、`ErrBadSignature`、`ErrRevoked`、`ErrTampered`、`ErrUnknownKey`、`ErrTooLarge`、`ErrBadID`、`ErrNoSecrets`、`ErrHung`……），由 `bind_plugins.go` 映射成 catalog 文案，不把 `err.Error()` 拼进本地化句子。

### 7.6 多语言

- 看懂自己的文案按 AGENTS.md 写进 `apps/kando/locales` 的五份 catalog，前缀用 `settingsPlugins…`、`plugins…`，然后跑 `go tool nsgo i18n`，把 `catalog_gen.go` 一起提交。
- 插件提供的文字来自清单的 `Text` map，只在 `LabelFunc`、`ContentFunc`、`core.Dynamic` 这类绑定里用 `Text.In(i18n.Language(cx))` 解析（NSGo 的语言是字符串，见 `i18n/i18n.go:104`），这样切换语言时会跟着变，满足 `locales/follow_test.go`。找不到当前语言就沿 `i18n.Parent` 回退，最后用 en。
- 复数只支持 `i18n.Cardinal` 的 one/other，第一版不做。
- 插件文字一律当纯文本显示，并限制长度。
- 手机端：不显示插件，只同步插件的产出（glossary 已经体现在字幕里；标注和读音原打算作为普通数据同步，P1.4 还没有做，见 E2）。查词结果经 `vocabResolve` 由桌面算好，回给手机。不加新的 `Platform` 值，不改协议版本。

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
- `apps/kando/plugins/dict/`：解析 Yomitan（term bank 的第 1 版和第 3 版格式，含 structured-content）、CC-CEDICT（繁简两种写法都能查，词条按点的那种写法显示，拼音改成声调符号）、TSV（`词条<TAB>读音<TAB>释义`，或没有读音的两列），包装好后第一次打开时在插件目录的 `.index/` 下生成排序索引 `<id>_<哈希前 16 位>.idx`，查词时在磁盘上二分查找，不整份读进内存，用完可以 Close。建到一半中断留下的临时文件（`<索引名>.build-*`）由同一内容的下一次 Open 或这个包其他内容的 Forget 删掉。查词时读到损坏的索引，app 关掉它并立刻重建；打开失败的原因可能是暂时的（读写文件出错，`dict.Retry`）时，下一次同步再试，词典本身读不了（`dict.ErrEmpty`、格式错误）时等内容变了再试。
- `apps/kando/plugins/wordlist/`：解析词表，word → tier（整份读进内存的 map，按小写查）。
- `apps/kando/plugins/annotate/`（P1.4）：把一部影片的各行原文算成 `annotate.Mark`（读音、等级色、已学词下划线），只依赖 `lookup`、`plugins/wordlist` 和 `segment`。
- `apps/kando/plugins/script/`（P3）：
  - Trust：OpenTrust，每次启动 AddRoot 内嵌根和 dev 根，吊销。
  - Verify，预读沙箱里比对 `kandoManifest()`。
  - `Limits()`：以 `tsx.DefaultLimits` 为底。
  - 构造 Sandbox 的唯一函数 `script.Sandbox(id, signer string, grant plugins.Grant, host *core.Host) tsx.Sandbox`：Grant 从零构造成 `&pkg.Capabilities{Network, Host}`，Storage 是 `plugin.<id>.<signer>`，Limits 来自 `Limits()`。
- `packages/media/translate.go`（P2.0）：`ChatWith`、`ChatRequest`、`ChatReply{Text, Usage}`；`Chat` 改成调它，行为不变。
- `packages/core/segment`：新增 `Annotated(lang, line) []Morph`（P1.2）；完整 features 词典按需加载，`Words` 不变。
- `packages/core/videosites_validate.go`（P4.0）：`ValidateVideoSite`、`ValidateVideoSites`，返回包着 `ErrVideoSite` 的 error，CLI、测试和运行时共用。原来 `browse/videos_test.go:89` 的 `shared` 正则挪到这里（不导出），那个测试改成调 `ValidateVideoSites`。同一个文件旁边的 `videosites.go` 加了 `VideoMatcher`，和 `VideoLink` 共用匹配代码。

### 8.2 根目录 package main 的新文件

- `state_plugins.go`：pluginsUI、savedPlugins、pluginsHooks、pluginsOverlay、pluginTerms、pluginPresets、读目录、inbox、安装、开关、卸载；pluginKeys 和 pluginCall 到 P3。`llmPresets` 在 P0.1 已经放在 `settings_ai.go`（AI 服务那块的文件），P1.1 在那里加上 `a.pluginPresets(m)`，`openLLMKeyPage` 遇到插件预设时改调 `pluginsOpenKeyPage`。插件预设的名称要用调用方的语言，而 `locales.Messages` 原来不暴露它的语言，所以在 `locales/languages.go` 加了 `Messages.Language()`。
- `state_plugins_marks.go`（P1.4）：标注的键、后台计算、`CueMarks`，以及 `settings.MarksApp` 的 getter 和 setter。
- `bind_plugins.go`：settings.App 和 film.App 需要的插件 getter 和 action（`live.Func`、`live.Do`），错误到 catalog 的映射，每个插件自己的 `core.Host` 和 handler（读 app 状态、防剧透裁剪、LLM 代理、credentialFetch）。
- `plugins_test.go`、`screenshot_plugins_test.go`。

### 8.3 共享文件（各一行）

- `main.go`：`app` 加字段 `plugins pluginsState`，`newApp` 加一行。
- `settings_file.go`：`savedSettings` 嵌入 `savedPlugins`，`clean` 加一行。
- `events.go`：`areas()`（`:54`）在 settingsHooks 之后加 `a.pluginsHooks()`。
- `keys.go:83`：`slices.Concat` 加 `a.pluginKeys(cx)`（P3）。
- `state_overlay.go:23`：`overlays()` 加 `a.pluginsOverlay(cx)`。
- 另外 `state_settings.go` 的 `settingsWindow` 给 `settings.Window` 加一个 `Plugins: settings.Plugins{App: a}` 字段（照 `Account` 的先例），P1.4 再加 `Marks: settings.Marks{App: a}`，`bind_settings.go` 的 `Presets()` 改成 `a.llmPresets(a.messages())`。

### 8.4 各接点的最小改动

- `settings/presets.go`：`PresetByID` 和 `ProviderID` 改成接收预设列表（P0.1），连同 E4 列出的全部调用处；`settings_ai.go` 的 `savedLLM.clean`（P0.2）。
- `state_vocab.go:215` 的 LookUpWord 和 `state_phonesync_study.go` 的 `phoneStudy.LookUp` 都改成调 `vocabResolve`（P1.3）。
- `state_glossary.go:119`、`:151` 的 `FilmTerms`（P1.1）。
- `bind_settings.go` 的 `Presets()` 绑定改成返回 `a.llmPresets(a.messages())`，设置 › AI 的下拉框按来源分组（P1.1）。
- `packages/core/segment` 的 `Annotated`（P1.2）。
- `film/subtitle.go` 的 lineWords 加 marks（P1.4），`film/subtitle` 叶子包里加 `Mark` 和四个 tone 常量，`film.App` 加 `CueMarks(i int) []subtitle.Mark`；`settings/window.go` 的 `layout`、`page` 多传一个 `marks` 节点，`settings/look.go` 的 `look` 把它画在最后。
- `settings/sections.go` 的 `var sections` 加一行，`settings/window.go` 的 `page()` 加一个 `SectionPlugins` 分支，照 `Window.Account` 的先例注入 `Window.Plugins`，`settings/sectionid` 加 `Plugins`。
- `packages/media/translate.go` 的 `ChatWith`（P2.0）。
- `subexport` 的格式列表（P3）。
- `saved/targets.go`（P2）。
- `cli/cli.go:148-165` 加一行。
- `remote/remote.go` 和 `remote_api.go` 加只读的 `plugins.list`。
- `state_browse.go` 的 `browseNotVideo`、`browseList`、`browseEnqueue`、`browseHeard`，和 `state_browse_buttons.go` 的 `browsePageAsk`、`browsePageAdd` 接 `pluginSites`，`browseSitesChanged` 换页面脚本并重建 web view（见 E8）；`queue.go` 的 `queueRun`、`queuePipeline` 和 `queue_remedy.go` 的 `FailureRemedy` 对 `core.Job.PluginSite` 的任务不借用登录（P4）。
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
- **`dict` 和 `wordlist`**：用小型样例词典验证索引构建、二分查找、原形查询、CC-CEDICT 的拼音，以及 Close 之后 Swap 能成功；词表的注释、未知 tier、大小写、重复词和空词表。
- **`annotate`**（P1.4）：假名注音去掉前后相同的假名；日语按原形查词表和生词本；没有大词典时只按表层形；中文拼音取第一个有读音的词典；别的语言的词表不画线。`film` 包的 `lineSpans` 测读音 view、等级的彩色实线（字的颜色不变）、已学词的虚线、越界标注被丢弃和描边副本只带读音，以及标注的边界落在词中间时只在标注自己的字下面画线。app 层测换词（词数不变）后下划线跟着换，键回到原值时丢掉过期的计算，以及译文写进来（字幕文件改写、重读）时标注一直在、不重算，原文改了才重算、新标注到之前原文没变的行照旧显示旧的、改了的那行不显示；新的一份计算会停下前一份。
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
  - 预设解析：`ProviderID(a.llmPresets(a.messages()), base)` 对插件地址返回 `pack.<id>`。
  - in-flight 自愈：预先放一条残留的 in-flight 记录再启动，那个插件被标为 `hung` 并停用。
  - `llm.chat` 的 `ErrHostTimeout` 不计入熔断。
- **app 级测试（nsgotest）**：`newTestApp(t, p)` 在 `f.home` 下放样例包。
  - 用 `settingsOpenAt(a, tt, sectionid.Plugins)` 进入设置页，验证列表、开关、安装确认（含高风险组合和凭据主机行）、问题区和错误文案。
  - 离线、无 AI 时，在桌面点词出现词典释义；手机同步路径（`phoneStudy.LookUp`）同样返回词典释义；词典顺序生效。
  - glossary 包的条目不覆盖用户的 Locked 条目，`FilmTerms` 也包含包的条目。
  - 选择 pack 预设后，服务确实存在 `pack.<id>` 下；拿掉包再跑 `savedLLM.clean`，key 还在；`savedPlugins.clean` 保留缺失的 id。
  - 标注只在 `playing` 或 `afterChange` 发现新的影片键时计算一次，任务进度触发的 `shelfChanged` 不会让它重算；只是译文变了也不重算。
  - `quitStarting` 之后所有插件都已 Close。
  - 插件命令进入菜单和面板，和内置按键冲突时插件的按键被清空（运行时去重，和 `keys_test.go:266` 的唯一性测试并存）。
  - 切换语言后插件名跟着变（follow_test 的规则）。
  - 在 CLI inbox 放一个文件，app 只弹确认，不直接安装。
  - P4：`browserExtVideo`、remote 的 `browser.*` 和 `sites` 都不认插件站点，网页按钮的两个入口只认已同意的插件站点，且用宿主的匹配器重新判断，CLI 和 MCP 的 `add` 不把它当站点包的任务；站点包任务的 Tools 不带 Cookies 和 SiteCookies；每个域名单独同意（`TestPluginSite*`）。
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

- PR 1.1：设置 › 插件页、安装确认、拖到插件页上安装、CLI inbox（`kando plugin install`），加上 glossary 和 ai-service 两种 kind（接入 E3 的两处和 E4 的 `llmPresets`、https 规则、下拉框里排在后面并写明 host），以及 savedPlugins、pluginsHooks（restore/save/ready/becameActive）、五份 catalog、截图。这是第一个对用户可见、范围最窄的 PR。
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
- PR 3.6：签名运维：插件根密钥、`go tool nsgo certify/revoke` 的发布流程、吊销列表随 appcast 发布、app 内的开发者模式（生成密钥、打包并安装文件夹）、CLI 的 `kando plugin install`（`list` 和 `validate` 在 P0.3 已经有了）。
- PR 3.7：官方参考插件 `kando.ankiconnect`，同时作为示例和 CI 夹具。

### P4 站点包

- PR 4.0：`core.ValidateVideoSite`。
- PR 4.1：site kind 和单独的 `pluginSites` 匹配器，只接内置浏览器（工具栏的「下载视频」和网页按钮的两个入口，见 E8），逐域名同意，不带浏览器 cookie，不支持 sign-in。加测试锁住 socket、浏览器扩展、remote 不认插件站点。设置页说明浏览器扩展不覆盖插件新增的站点。

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
5. **site 包**：已经改成独立的 `pluginSites`，不放宽全局闸门；剩下的风险是 JS 正则的校验漏了嵌套量词时会让页面卡顿，以及用户对「浏览器扩展不覆盖插件站点」的预期要讲清楚。插件正则现在也写进内置浏览器的页面脚本，在网页的 JS（回溯引擎）里跑，所以这条风险是真实的；缓解全靠 P4.0 的规则：不允许重复里套量词或多选、每条最多两个无上限量词、不超过 256 字节，链接最长 2048 字节，最坏是平方级，约几百万步。规则只在 Go 里校验，`ScriptWith` 写进页面前再校验一次。浏览器扩展不认插件站点（扩展的文件由 `go generate` 生成、随版本发布，装不进用户的包），设置页的说明写的是「浏览器扩展不支持插件网站，请在看懂内置的浏览器里使用」。另一个代价：同意的网站一变，内置浏览器的 web view 就重建，后退历史没了（nsgo#840）。
6. **clean 丢数据**：`savedLLM.clean` 和 `savedPlugins.clean` 一旦写错，插件暂时缺席时就会丢用户的 key 或选择。P0.1、P0.2 和 P1 都有测试锁住。
7. **token 费用**：task 和 `llm.chat` 花的是用户的钱，默认预算必须保守，用量必须可见；服务商不返回 usage 时只能估算。
8. **离线用户收不到吊销列表**：长期离线时，已被吊销的纯本地插件可能继续运行，这是有意的折中。本机程序还可以通过删改 trust.json 清掉吊销和防回滚（6.7 节）。
9. **凭据**：Linux 没有 Secret Service 时，凭据类插件不可用；Windows 和 Linux 的凭据库对同用户开放（6.7 节）。
10. **i18n**：插件文案不经过 `catalog_test` 检查，缺语言时回退到 en，翻译质量取决于作者。
11. **手机版本滞后**：插件产出的数据同步到手机时，ledger 必须向后兼容。
12. **P2.1 的重构**会碰 5 个以上文件，要先给 owner 看计划。P0.1 按 E4 列出的调用处直接做，行为不变，由测试锁住。
13. **P1.2 的内存**：完整 features 词典会增加常驻内存，所以先测量，达标（12.2 第 5 条）才只在桌面按需加载，否则走只用词典包的退路。实测增量约 92 MiB、加载约 0.6 s（第 15 节），已按达标处理。

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
4. ~~**是否做站点包（P4）**~~：owner 决定做，P4.0、P4.1 已完成（第 15 节）。
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
| P0（0.1–0.3） | 0.1 `PresetByID`/`ProviderID` 接收预设列表；0.2 `savedLLM.clean` 保留 `pack.` 服务；0.3 `apps/kando/plugins` 骨架（Parse、Validate、Stage、Swap、Remove、Scan、最小 state.json）和 `kando plugin validate`、`list` | `feat/plugins-foundation` | 已完成 |
| P1.1 | 设置 › 插件页、安装确认、拖到插件页上安装、CLI inbox，glossary 和 ai-service 两种 kind，savedPlugins、pluginsHooks | `feat/plugins-data-packs` | 已完成 |
| P1.2 + P1.3 | `segment.Annotated`（含测量和路线选择），dictionary kind、`plugins/dict`、`vocabResolve`、查词卡来源、词典排序 | `feat/plugins-dictionary` | 已完成 |
| P1.4 | wordlist kind、`plugins/wordlist`、`plugins/annotate`、逐词标注（注音和等级色、已学词下划线）、设置 › 字幕外观 的开关 | `feat/plugins-wordlist` | 已完成 |
| P1 收尾 | 等级改成彩色粗下划线、已学词改成虚线；卸载遇到更新版本的 state.json 时先拒绝，删掉后没存上 state.json 算卸载好；Scan 的哈希缓存；手机查词卡显示来源（协议加 `WordSource`）；查词卡的离线词典提示和「词典没有这个词，问了 AI」；插件页词典分组的截图 | `feat/plugins-polish` | 已完成 |
| P1.5 | 官方词典包 | — | 不做（12.2 第 2 条） |
| P2 | 学习任务、导出模板、翻译风格 | — | 未开始 |
| P3 | 脚本插件 | — | 未开始（等 12.3 第 1、3 条） |
| P4.0 | `core.ValidateVideoSite`、`ValidateVideoSites`、`VideoMatcher` | `feat/plugins-sites` | 已完成 |
| P4.1 | site kind、`pluginSites`（工具栏的「下载视频」和网页按钮）、逐域名同意、站点包任务不借用登录 | `feat/plugins-sites` | 已完成 |
| P5 | 按需单独立项 | — | 未开始 |

P1.2 的测量结果（Ubuntu 编译机，x86_64，Go 1.27.4，kagome-dict/ipa v1.2.6；每次读 `/proc/self/status` 前先 `runtime.GC()` 和 `debug.FreeOSMemory()`，各在一个新进程里量，量完的测试没有提交）：

| 情形 | 加载时间 | RSS（VmRSS） | 峰值（VmHWM） |
|---|---|---|---|
| 空进程 | — | 8.3 MiB | 8.3 MiB |
| 只读 `DictShrink()`（`Words` 现在的做法） | 0.45 s | 50.1 MiB | 86.4 MiB |
| 读过 `DictShrink()` 后再读 `ipa.Dict()` | 0.60 s | 141.7 MiB（+91.6 MiB） | 195.4 MiB（+109 MiB） |
| 只读 `ipa.Dict()` | 0.59 s | 113.0 MiB | 147.7 MiB |

`ipa.Dict()` 在 `DictShrink()` 之后读时是另读一份（`DictShrink` 只在完整词典先读的情况下复用它），所以增量按第三行算：常驻约 92 MiB、峰值约 109 MiB，加载约 0.6 s，都在 12.2 第 5 条的门槛（200 MiB、3 s）以内。选择：用完整 features 词典，只在桌面、只在查词用到已启用的日语词典包时（P1.4 起还有日语词表包）由 `segment.Annotated` 第一次调用时读入；手机不调 Annotated，分词照旧只用 `DictShrink`。

与正文的偏差：实现过程中某个接点挪了位置或做了简化，就同时改正文对应的一节和这一段，写明改了什么、为什么。

- P0.1（已完成）：`llmPresets` 带一个语言参数 `m`，因为引导页（`locales.For(cx)`）和反馈 issue（固定简体中文）读预设名称用的语言和 `a.messages()` 不同，不带参数就会改变它们显示的名称；`llmPresets` 放在 `settings_ai.go` 而不是 `state_plugins.go`，因为 P0.1 时还没有插件块；`feedbackTranslator` 改成接收预设列表。见 E4 和 8.2 节。
- P0.2（已完成）：`savedLLM.clean` 保留 id 以 `llmPackPrefix`（`"pack."`）开头的服务，没有地址的仍丢掉。P0.2 时还没有插件预设，测试只锁住 clean 本身；「插件在场时存进 `pack.<id>`、拿掉后 clean」的端到端测试推到 P1.1。见 E4。
- P0.3（已完成）：`apps/kando/plugins` 按 8.1 节分成 `manifest.go`、`validate.go`、`names.go`、`install.go`、`hash.go`、`state.go`、`errors.go`，CLI 是 `cli/plugin.go`。和正文的出入如下，各节已同步改过：
  - 保留名单是包里的静态列表 `plugins.ReservedNames`，不是从 `settings.Presets` 现取，因为 `plugins` 不能导入界面包；由根目录的 `plugins_test.go` 锁住它和 `settings.Presets`、`appName` 一致（6.4）。
  - `health.go` 和 `audit.go` 没有写，按 12.2 第 4 条推到 P3.1。届时注意：`feedback` 是界面包（反馈对话框在里面），`plugins` 不能导入它，audit.log 要经 `feedback.Scrubber` 清洗，就得由调用方传入清洗函数，或者把 Scrubber 挪进一个非界面包。
  - 清理残留的 `.staging-`、`.old-` 不另设入口，由每次 `Stage` 开始时顺带做；`Swap` 换下的旧目录当场删除，删不掉才留给后面的 `Stage`（5.1、6.5）。
  - `Swap` 在 Windows 上的重试和待替换标记推到 P1.3，P0.3 时还没有任何打开的句柄（5.1）。
  - 从文件夹安装和 `Validate` 都跳过文件管理器写的 `.DS_Store`、`Thumbs.db`、`desktop.ini`，已安装包的目录被访达打开过也照样有效；zip 不能有目录条目，manifest 必须在顶层；文件名的字符集、设备名和其他字段规则补进了第 4 节。
  - id 在所有 kind 之间唯一：`Swap` 遇到别的 kind 已有同一个 id 就返回 `ErrIDTaken`，`Scan` 把重复的报成 Issue（第 4 节）。
  - `Swap` 接收 `*Pack`，成功后把 `p.Dir` 改成安装好的目录，`Stage` 返回的 staging 路径那时已经不存在（5.1）。
  - zip 的中央目录在 `zip.NewReader` 读取时限 1 MiB，嵌套 zip 的目录条目也计入 64 个条目的上限，嵌套 zip 里损坏的文件算 `ErrBadArchive`（第 4 节）。
  - `State` 多了 `Revision`（每次 `Save` 加一）；state.json 读不了、没有 format 或格式比本版新时所有包按停用处理，格式更新的文件不覆盖；没有条目的包按启用处理。损坏后的第一次 `Save` 会让没有条目的包重新启用，P1.1 要在保存前给每个包补上停用条目（5.1、6.2）。
  - `kando plugin list` 在 P0.3 一起做了，它不连 app，直接读插件目录（5.4）；P3.6 只剩 `install`。
  - CI 的 Test（Linux）加了一步 `Fuzz the plugin packs`，`FuzzParse` 和 `FuzzUnzip` 各跑 20 秒（第 9 节）。
  - `Scan` 每次都完整校验并哈希每个包，dictionary 还要把嵌套 zip 读一遍。P1 收尾时加了按文件名、大小和修改时间的哈希缓存（6.5）。
- P1.1（已完成）：设置 › 插件页（`settings/plugins.go`）、安装确认、CLI inbox、glossary 和 ai-service 接入。和正文的出入如下，各节已同步改过：
  - app 一侧的类型叫 `pluginsUI` 而不是 `pluginsState`，这样 `main.go` 的 `app` 结构体只多一行、不让 gofmt 重排整段对齐；`savedPlugins` 只有一个 `PluginOrder`（所有见过的 id，按出现顺序，决定列表顺序和术语包之间谁先），取代 `PluginDictOrder`，P1.3 让词典组可以重排，P1.4 再加标注开关（5.2）。
  - `PluginPresets()` 改成 `pluginPresets(m)`：名称要用调用方的语言，为此 `locales.Messages` 加了 `Language()`（8.2）。插件预设排在全部内置预设之后，下拉框行写成「插件 · 包名 · host」；`ui.Select` 没有带标题的分组，留了 `TODO(nsgo)`（E4）。
  - 「获取 API Key」对插件预设先弹系统提示框（`base.ShowAlertDialog`）显示 host，不是画在页面里（E4）。
  - 启动时先认出的服务商在插件目录读完后会重新认一次（`pluginsFollowLLM`），开关、安装、卸载之后也一样；当前选的服务还在就不换，「自定义」不换（`llmProviderFor`）。正在用的包服务消失后记作 gone，它的地址和 key 只存进 `pack.<id>`，不会盖掉用户的自定义服务；`llmRemember` 改用 `llmProviderFor`，`settings` 加了 `SameEndpoint`（E4）。
  - 术语包按目标语言严格过滤、来源语言已知时才过滤，经 `plugins.AddTerms` 用 map 合并（`core.MergeTerms` 是平方复杂度）。包的条目带新加的 `core.OriginPack`，在播放器的「术语」标签里只读，说明行指向 设置 › 插件；「术语」标签只列影片原文字幕里出现的包条目，翻译仍用全部条目（E3）。
  - `pluginsSameLanguage` 挪进 `plugins` 包，改名 `plugins.SameLanguage`，和 `AddTerms` 一起放在 `plugins/terms.go`。
  - 点「安装」之后才失败的，确认框不关，换成失败视图等用户点「好」；确认框随界面语言重建（7.2）。看 inbox 用单独的 `inboxBusy`，不让开关在切回前台时失灵，开关存完后再看一次（5.4）。读目录（`pluginsScan`）在后台读 state.json，app 自己正在改或刚改过 state.json 时（`stateGen`、`saving`）保留 app 手里的状态，只取读到的包和问题，免得较旧的读取把刚做的开关改回去（5.1）。
  - 拖放安装只在 设置 › 插件 页上做，主窗口的拖放仍归片库（6.5）。
  - `settings/sections.go` 的 `sections` 不改成函数，只加一行；插件页由 `settings.Window.Plugins` 注入，读自己的 `settings.PluginsApp`，不扩大 `settings.App`（7.1、8.4）。
  - 安装确认在设置自己的窗口里由插件页画，其余情况由主窗口的 `pluginsOverlay` 画（7.2）；卸载确认在详情里展开，不是对话框（7.1）。
  - CLI inbox 由新加的 `plugins.Drop` 和 `plugins.Inbox` 实现，app 在 `ready` 和 `becameActive` 时检查（5.2、5.4）。
  - Validate 已经接受 dictionary 和 wordlist（P0.3），所以这两种包现在也能装、能开关，但在 P1.3、P1.4 接入之前不起作用。
  - state.json 损坏后第一次保存前，给每个没有条目的包补上停用条目（`pluginsNextState`），损坏前关掉的包不会被重新打开（5.1）。
  - 反馈 issue 的翻译服务一行只认内置预设，不会写出插件包名（E4）。
  - 截图场景 `TestScreenshotsPlugins` 有插件页（两个包）和主窗口上的安装确认两张；详情页、查词卡来源和带注音的字幕留给 P1.3、P1.4。
- P1.2（已完成）：`segment.Annotated` 和 `segment.Morph`。和正文的出入如下，E1 已同步改过：
  - `Words` 原来的切分循环（`cutRuns`）改成在新的 `cutMorphs` 上包一层，日语的合并规则抽成 `japaneseMorphs`，`Words` 和 `Annotated` 共用；`segment_test.go` 的 `TestWordsAreCutAsBeforeAnnotated` 用改动前的代码跑出的切分结果锁住 `Words` 的输出逐字不变。
  - 合并词的读音是各 token 读音连起来（片假名，`イッタ`），不是原形的读音；原形的读音要从词典包取。
  - `Annotated` 会等词典读完（`Words` 不等），只能在后台调；`lang` 为 `ja` 时只有汉字的行也按日语切。
- P1.3（已完成）：dictionary kind 接入查词。和正文的出入如下，E1、5.1、5.2、7.1、8.1 已同步改过：
  - `plugins/dict` 是具体类型 `*dict.Dict`，不是接口，`LookUp(word, base, reading)` 不带 ctx（几次小的磁盘读）；只有一个实现，到 P3 有脚本查词来源时再看要不要接口（5.1）。
  - 索引放在插件目录的 `.index/`，不放进包目录（包目录里多一个文件会让 `Validate` 拒收、哈希改变）；按 `<id>_<哈希前 16 位>` 命名，内容变了就重建，旧的删掉，卸载时一起删。旧索引不在 `dict.Open` 里删，而是每次打开结束后在引擎 goroutine 上决定保留哪一份（当时装着的那份内容的索引，包目录已经不在就一份不留），再在后台 `dict.Forget`：这样建索引期间包被替换或卸载时，晚结束的那次打开既不会删掉新内容的索引，也不会留下自己那份几百 MB 的孤儿索引；同一份内容正在打开时再开关一次，不会再起第二次构建（`pluginsUI.opening`）。建索引发生在安装（或换内容）后第一次打开时，在后台，不在 `Stage` 里（5.1、8.1）。
  - 因为长期打开的只有索引，`Swap` 换包目录时不受句柄影响，Windows 的重试和待替换标记不需要，没有做（5.1）。
  - 打不开的词典（比如没有一条能读的词条，`dict.ErrEmpty`）列在插件页的问题区，同一内容不再重试。
  - `vocabResolve` 拆成引擎 goroutine 上的一半和后台的 `lookup.Ask.Answer`（不碰 app 的部分放进 `lookup` 包，见 E1）；缓存只存 AI 的回答，词典的结果不进缓存；`to` 不是回答语言的词典只在没有 AI 时才答（E1）。
  - 日语词先用 `segment.Annotated` 取原形和读音，按原形查；只在真的会查词典时才调 Annotated（有回答语言的日语词典，或者没有 AI 时有别的语言的日语词典），所以只装了英文释义的 JMdict、又配了 AI 时，查词不会去读那份约 92 MiB 的完整词典。读音是点到的词的读音（片假名，和词典的平假名比较前统一成平假名），只在按点到的词本身命中时用来挑同形词；按原形命中时（`行った` 读 イッタ，原形 `行く` 读 いく 或 ゆく）不挑，原形的各个读音都列出来，因为 kagome 的 IPA features 里没有原形的读音。
  - 词典排序的上移、下移不重建列表：列表的 `core.Dynamic` 只按包的集合（排序后的 kind/id/hash）重建，每组的行用 `core.Each` 按 id 保留并移动，按钮能不能按从绑定里读，所以按过的按钮还在、焦点不丢。
  - 词典排序用上移、下移按钮，不是拖动（7.1）。
  - 查词卡的来源只在桌面显示；手机同步协议没有来源字段，手机端暂不显示，JMdict 这类 CC-BY-SA 词典在手机上的署名要等协议加字段（E1）。P1 收尾时加了 `psync.WordSource`，手机也显示了。
  - 查词卡没有 AI、词典也没查到时的提示（`vocabLookupNoAI`）改成同时指向「AI 服务」和「插件」里安装词典（12.2 第 2 条）。
  - 截图场景 `TestScreenshotsPlugins` 加了 `plugins-lookup`（词典回答的查词卡）；插件页里的词典分组和详情页还没有截图场景。P1 收尾时加了 `settings-plugins-dictionaries`（两个词典包、上移下移按钮、展开的详情），截图辅助 `shootSettingsPressed` 在截图前按下给定的控件。
  - 没有拿真实的 JMdict、Jitendex、CC-CEDICT 全量文件测过（测试用小样例）。第 4 节 64 个条目的上限对条目多的 Yomitan 词典（带图片的 Jitendex 之类）可能不够，建索引的耗时和峰值内存（所有键先放在内存里排序）也还没量；有人装了大词典报上来再调。
- P1.4（已完成）：wordlist kind 和逐词标注。和正文的出入如下，E2、第 4 节、5.1、5.2、7.4、7.6、8.1–8.4、第 9 节已同步改过：
  - `Mark` 去掉了 `Tip` 和 `Tier`：`core.Span` 没有悬停提示，也没有别的地方读 tier 的 id；加了 `ToneAccent` 等四个常量。计算放在新的非界面包 `plugins/annotate`，它的 `Mark` 和 `subtitle.Mark` 字段相同，app 用类型转换，`film` 照旧不导入插件包。
  - 缓存只留当前影片一份，不是按键的 map；键除了「视频 + 字幕哈希」，还包括说话语言、三个开关、选中词表和中文词典的内容、是否读日语大词典和生词本的内容指纹（不是词数，换一个词也重算），任何一项变了都重算一次；键回到已有标注的键时丢掉正在算的那一份。字幕哈希只含每行的起止时间和原文，不含译文，只在播放器换了字幕切片时重算。重算期间 `CueMarks` 照旧给出旧标注（视频和行数相同、这一行原文没变时），不再核对切片身份；除了 `playing`，`afterChange` 也调 `pluginsMarksFollow`，暂停时改字幕或生词本也会重算。
  - 开关放在 设置 › 字幕外观 的「逐词标注」卡片，默认都关；由 `settings.Window.Marks` 注入，读自己的 `settings.MarksApp`。
  - 已学词用生词本里同一说话语言的词集合判断，不调 `WordIndex`（它每次线性扫一遍）。
  - 日语读音只在启用了日语词典包、或选了日语词表时才有（读大词典的门槛，见 E1）；中文拼音只来自中文词典包。词表只给语言和它的 `language` 相同的行画线，说话语言不知道、文字又看不出语言的行（比如没标语言的英文片）不画线。
  - `lineSpans` 除了词和读音的边界，也在画线标注的两端切开（只在最上面那一份；描边副本不画线，不切）：标注来自 `segment.Annotated`，词来自 `segment.Words`，两者不一定切在同一处（比如只有汉字的日语行，`segment.Words` 按中文切）。
  - 描边模式下，读音让行的排版变了，所以描边副本和撑开大小的那一份也带同样的读音 view；没有读音时副本照旧画 Content，不切 spans。
  - 手机端没有标注（协议没有字段）。
  - 词表读不了（`wordlist.ErrEmpty` 或读文件失败）时列在插件页的问题区，同一内容不再重试。
  - 截图场景 `TestScreenshotsPlugins` 加了 `plugins-marks`（日语原文带假名注音和等级色）。Linux 上的 nsgotest 不出图，这一张只在 macOS 的截图任务里画得出来，本次只在 Linux 上断言了标注本身，没有看过实际画面；读音 view 的竖直对齐（`VerticalAlignTextBottom`）和描边副本的模糊滤镜是否作用到 attachment 上，要在 macOS 截图里核对。
- P1 收尾（已完成）：等级的标注、卸载、Scan 的速度、手机查词卡的来源、查词卡的两句词典提示（7.4、7.5）和插件页词典分组的截图。和正文的出入如下，E1、E2、6.5、7.1、7.2、7.4、7.5、第 9 节已同步改过：
  - 等级不再改字的颜色，改成在词下面画一条该等级颜色的粗实线；已学词从实线改成虚线，两者都有时是等级颜色的虚线（owner 的决定，E2）。设置里的「按等级着色」改名「按等级画线」，说明写明粗线和虚线（7.4）。NSGo 的 `core.Style` 本来就有下划线的颜色、样式和粗细，三个桌面平台都画，没有缺口。描边模式下线只画在最上面那一份，不带黑色描边；粗细和位置按字号的比例，没有在 macOS 截图里看过实际效果。
  - 卸载遇到更新版本的 state.json 时在删除之前就拒绝；目录删掉后 state.json 没存上的，插件页说「已完成，但……」，不说失败。安装后没存上也改用这一句（新的 `pluginsErrStateUnsaved`），原来说的是「无法读写这个插件包」，和实际不符（7.1、7.2）。
  - `Scan` 的哈希缓存放在插件目录的 `.hashes.json`，按 `<kind>/<id>` 存每个文件的名称、大小、修改时间（纳秒）和哈希；`Validate` 本身不变，`Scan` 走内部的 `validate(dir, cache)`（6.5）。
  - 手机查词卡的来源：协议的 `WordEntry` 加了可选的 `Source`，不升版本；名称和 attribution 用桌面的界面语言，因为桌面不知道手机的界面语言；手机缓存的旧查词结果没有来源，照旧不显示（E1）。手机截图场景 `player-word` 带了「由 AI 服务解释」一行。
  - 没做的 P1 缺口：手机端没有逐词标注（要加协议字段和手机的字幕层，不小）；真实的 JMdict、Jitendex、CC-CEDICT 全量文件和 64 个条目的上限没有测（要真实数据和 owner 定上限）；读音 view 的对齐和描边、彩色下划线的实际画面要在 macOS 截图任务里看；`ui.Select` 没有分组（`TODO(nsgo)`，等上游）。
  - 离线词典的提示只看装没装这种语言的词典包，不看开没开：装了却关掉是用户自己的选择，提示「去装一个」不对。插件目录还没读完时不提示，免得把已装的词典当成没装。提示作为卡片底部单独的一行加一个「打开插件设置」按钮，按了先关卡片再打开设置 › 插件（`LookupDictionaries`）。
  - 评审后的修正：「词典没有这个词」不再算上查词出错的词典；手机查词卡的「由 … 上的 AI 服务解释」改用回答时桌面发来的名字（`WordSource.Desktop`，可选字段，不升协议版本）；`State.Save` 写之前重读磁盘上的格式；拒绝卸载时的提示和 state.json 的说明不再重复；`StateUnsaved` 挪进 `plugins` 包。
- P4.0（已完成）：`core.ValidateVideoSite` 和 `ValidateVideoSites`（`packages/core/videosites_validate.go`）。和正文的出入如下，E8、8.1 已同步改过：
  - 比正文写的更严：字符集之外，反斜杠后面只允许 `\w`、`\W`、`\d`、`\D` 和转义的标点（`\1` 这类反向引用、Go 当 Unicode 类而 JS 当字母读的 `\p`，以及 `\b`、`\s` 都拒收）；大括号只能是紧跟在被重复的东西后面的 `{n}`、`{n,}`、`{n,m}`（每个数最多三位）；不允许以 `]` 开头的字符类（Go 把 `[]a]` 读成含 `]` 和 `a` 的类，JS 读成空类加 `a`）。
  - 「不允许嵌套量词」按 Go 解析出的语法树判断：被 `*`、`+` 或上限大于 1 的计数重复的部分里，不能再有任何量词（包括 `?`），也不能有多选分支（`(a|aa)*` 和 `(a+)+` 一样会让回溯引擎指数级回溯）；只是字符的多选（`(a|b)+`）会被 Go 合并成字符类，允许。另外每个规则最多两个无上限的量词（`*`、`+`、`{n,}`），三个相邻、能匹配同样字符的无上限量词会让回溯引擎对 2048 字节的链接做十亿级的尝试。
  - `path` 必须以 `^/` 开头、以 `$` 结尾，`value` 必须以 `^` 开头、以 `$` 结尾；`param` 是 1 到 64 个字母、数字、`_`、`.`、`-`；域名是至少两段的小写主机名，不是 IP；`not` 里的主机必须在域名下面且不重复；一组网站的域名不能相同，也不能一个在另一个下面。
  - 只看 `domain`、`not` 和 `videos` 的 `path`、`param`、`value`，不看 `menu`、`rows`、`titles`、`cut`，这几样只有页面脚本和扩展读。
  - 加了 `kcore.VideoMatcher`（`NewVideoMatcher` 先跑 `ValidateVideoSites` 再编译），`Find` 返回链接所在的网站域名和它是不是一个视频；`VideoLink` 改成和它共用 `findVideo`，行为不变（`browse/videos_test.go` 的 `TestVideoLinkFollowsParseLink` 照旧通过）。`VideoLink` 不认 `VideoMatcher` 的网站。
- P4.1（已完成）：site kind。和正文的出入如下，E8、3.1、7.2、8.4、第 9 节、10、12.1、12.3 已同步改过：
  - 先只接了工具栏的「下载视频」，同一分支上的后续提交补上了网页按钮（视频页上的「下载视频」和缩略图上的 +），做法和限制见 E8：页面脚本在 web view 创建时读一次（nsgo#840），所以同意的网站一变就重建 web view，载入原来的页面，历史记录丢失。网页按钮的询问和加入都在 Go 一侧用同一个匹配器重新判断（`browsePageSite`、`browsePageVideo`，`browse.ParseAskWith`、`ParseAddWith`），页面说什么都不信；插件站点的视频不再要求 `kcore.ParseLink` 认为是单个视频，和工具栏一样由包的规则决定。`TestScriptWithKnowsTheSitesGiven` 锁住页面脚本只含合法、已给出的网站，`TestPluginSitePageButtons` 锁住只有已同意的域名进脚本、Go 一侧的判断和重建。浏览器扩展生成的文件没有变化（`go generate` 后无差异，`.github/scripts/browser-extension.sh` 通过）。
  - 在同意的域名上，工具栏按包的规则判断哪些页面是视频（其余页面按钮变灰、不当播放列表），这些任务不借用登录。除了内置浏览器，没有任何入口多接受一个原本不接受的链接。
  - 站点包任务的标记是新加的 `core.Job.PluginSite`（包 id，`shelf` 的 `storedJob` 存为 `pluginSite`），不是只在构造 Tools 时判断链接：加入时同意过、之后撤回同意或卸载了包，这个任务照样不借用登录；反过来，CLI、MCP 加的同一个链接不是站点包的任务。除了 `Cookies`，`SiteCookies`（内置浏览器页面的 cookie，目前因 nsgo#112 还没接上）也一起清空。
  - 同意存在 `plugins.PluginState.Sites`，不是 5.1 节草图里 P3 才有的 `Grant.Sites`；`State.Format` 不升，旧文件照读。没有条目的包一个域名都没同意（`State.Allowed`），和「没有条目就是启用」的 `Enabled` 相反；state.json 读不了时也一个都不同意。开关包、换内容不会丢掉已同意的域名；生效的是「已同意 ∩ 当前包列出的域名」。到 P3 引入 `Grant` 时把它挪进去。
  - 安装确认框的复选框默认不勾；同 id 的旧版本已经同意、新版本仍然列出的域名预先勾上（`pluginsTickedBefore`），用户在框里看得到。
  - 校验错误多了两个：`ErrBadSite`（规则不安全、域名是公共后缀）和 `ErrSiteKnown`（看懂自己认识的网站）。站点包最多 8 个网站，每个最多 16 个链接规则、16 个 `not` 主机。
  - 设置页网站分组下的说明一行只说浏览器扩展不支持插件网站（网页按钮已经支持）。
  - 截图场景 `TestScreenshotsPlugins` 加了 `plugins-install-site`（站点包的安装确认，两个域名的复选框，勾了第一个）。Linux 上的 nsgotest 不出图，这一张只在 macOS 的截图任务里画得出来，本次只在 Linux 上跑通了场景本身。
