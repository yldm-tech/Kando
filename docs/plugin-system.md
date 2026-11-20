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

- 插件不能替换流水线阶段（下载、识别、校对、翻译、导出落盘），也不能在 `media.Run` 中间挂通用钩子。原因是 `Run` 的续跑和重做依赖文件语义（转写稿、`pendingHeard`、checkpoint、proofs），而 `apps/kando` 里有大量 `a.pipeline.(media.Tools)` 断言（非测试文件约 73 处）。`packages/media` 只做三处加法：P2.0 的 `ChatWith`（见 5.3），第 5 期可选的 `media.Tools.Engine` 字段，以及给失败标上步骤的 `media.StageError`（都见 2.3 的 E14），另外第 5 期的翻译来源导出了一个读 ctx 的 `SpeechLanguageOf`。这几处都不改 Pipeline、Run 的语义（`StageError` 的文字和以前的错误文字一样），也不碰任何断言。
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
- 接入位置：`apps/kando/film/subtitle.go` 的 `type lineWords struct` 加了 `marks func() []subtitle.Mark`（在 `subtitleLayer` 里为每行原文建一个 `core.NewMemoFunc`，经 `live.Func` 读 `film.App.CueMarks(cue)`，只在这行的标注变了时才让 spans 重算）。`withWords` 改成调 `lineSpans`：在词的边界和每个标注的边界处切开（画线的标注也切，因为标注和词的切分不一定一致），带读音的一段是 `core.Span.Attachment`（一个上面是小一半字号的读音、下面是原文的 view），点它照样查它所在的词；view 里那段原文量出自己的基线后挂到 view 下面，view 用 `VerticalAlignBaseline` 放在行的基线上，这样回退到 CJK 字体的字和它下面的线与旁边的字对齐（量到之前用 `VerticalAlignTextBottom`，原因见第 15 节「插件截图核对」）；其余标注不改字的颜色（字保持用户选的字幕颜色），而是在字下面画线：有 `Tone` 的画一条该等级颜色的粗实线（`TextDecorationUnderline`，`TextDecorationColor` 取 tone 的颜色，`TextDecorationThickness` 是字号的 9%，`TextUnderlineOffset` 是 14%），`Underline`（已学词）画虚线（`DecorationDashed`），两者都有时是 tone 颜色的虚线，所以等级和已学词永远不会看起来一样，彩色的线在任何字幕颜色上都看得见（owner 的决定）。NSGo 固定版本的 `core.Style` 有下划线的颜色、样式、粗细和位置（`core/style.go:220-230`），macOS、Windows、Linux 的文字后端都画得出来，不需要绕行。描边模式下原文要画十份（撑开大小的一份、九份黑色描边、最上面的一份），读音会改变行的排版，所以下面的九份和撑开大小的那一份也带上同样的读音 view（`withReadings`，不接点击，不画线），没有读音时它们照旧直接画 Content。
- 颜色不写字面量：`accent`、`warn`、`success`、`muted` 分别取 `ui.Colors` 的 `Accent()`、`Warn()`、`Up()`、`Muted()`（播放器那棵子树用深色主题）。
- `Mark` 定义在叶子包 `film/subtitle`（`Start, End int`、`Ruby`、`Tone`、`Underline`，字节偏移落在这一行原文里），所以 `film` 不导入插件包。计算放在非界面包 `plugins/annotate`，它有一个字段完全相同的 `annotate.Mark`，app 用类型转换 `subtitle.Mark(m)` 换过去，两边字段一旦不同就编译不过。一个词可以有两种标注：带 `Ruby` 的只盖住它的汉字部分（`食べた` 的读音 `た` 只在 `食` 上，词中间的假名在读音里只有一种对法时在那里切开，`山の上` 是 `山` 标 `やま`、`上` 标 `うえ`，对法不止一种时（`物の怪` 读 `もののけ`，`物` 可以是 `も` 也可以是 `もの`）照旧整段一个读音，前后相同的假名去掉），不带 `Ruby` 的在整词下面画线。
- 数据来源：日语的读音来自 P1.2 的 `segment.Annotated`（IPA 词典的片假名读音换成平假名），只在启用了日语词典包、或选了日语词表时才读那份大词典，否则日语没有读音，词表和已学词也只按表层形查；不读大词典时，凡是 `segment.Annotated` 会按日语切的行（说话语言是 `ja`，或者说话语言不是 `zh` 而行里有假名，比如英语片里夹一句日语）都改用 `segment.Words` 和小词典切，免得一部非日语影片因为一行假名就读进那份约 92 MiB 的词典；假名注音去掉前后相同的假名时，长音符 `ー`（含半角 `ｰ`）和半角片假名也算假名（`ラーメン屋` 读 `らーめんや` 只在 `屋` 上标 `や`），半角浊音符号 `ﾞ` `ﾟ` 不合并，遇到时读音盖住的范围宽一些；中文的读音（拼音）取自已启用的中文词典包（`a.pluginsDictsFor("zh", "")` 的顺序，第一个有读音的），同一个词一次计算里只查一次；等级来自选中的 wordlist 包，日语先按原形再按表层形查，只给语言和词表 `language` 相同的行上色；已学词来自生词本（`savedBook().Words` 里说话语言和这部影片相同的词（`annotate.SpokenIn`），按 `wordlist.Key` 小写比较），没有用 study 的 `WordIndex`，因为要一次判断整部影片的所有词。没有词频。
- 何时计算：插件块自己的 `playing` 和 `afterChange` 钩子（都是 `pluginsMarksFollow`；`afterChange` 管暂停时的改动，比如改了一行字幕、往生词本里加了词）每次只算一个可比较的键 `pluginsMarkKey`：视频、字幕哈希（`pluginsSubsHash`：只哈希每行的起止时间和原文（`film.SplitCue`），不含译文，所以翻译一批批写进来、或只改了译文时不重算；播放器换字幕时总是换一个新切片、从不原地改，所以只在切片变了时才重新哈希）、说话语言、三个开关、选中词表的 id 和内容哈希、已打开的中文词典的 id 和哈希、是否读大词典，以及生词本的内容指纹（`pluginsVocabHash`：和影片说话语言相同的词按 `wordlist.Key` 各取 `maphash.String` 哈希（本次运行的种子，不分配内存）后求和，再带上词数，与顺序无关；生词本是原地改的（`study.Book` 的 `slices.Insert`/`Delete`），改它的地方很多（生词本、收藏的句子、两种同步），没有用改动计数器，因为每处都得记得加一，漏一处下划线就不更新，所以每次报进度都扫一遍，只在下划线开着时才算；加词、删词、换词都会重算）。键和已有标注的键、正在算的键都不同时，才在后台 goroutine 里 `annotate.Lines`，算好经 `a.update` 存成不可变的切片；同一时刻只有最新的一份在算：开始新的一份、或键又回到已有标注的键时，前一份的停止标记（`pluginsUI.stop`，`annotate.Sources.Stop`）被置上，它在下一行之前停下，结果即使送回来也丢掉，免得它算完后盖掉当前的标注。它忘掉已读的词表（不再是选中的那份）时也报告有变化，因为插件页的问题区读它。开关在设置里改动时也调一次它；三个开关都关掉或影片没有字幕时丢掉标注。字幕文件被改写后播放器重读（`reloadOpenSubtitles`，字幕还没读完时 `subs.ready` 是 false）期间，它既不丢标注也不丢正在算的那一份，读回来的原文没变就直接沿用。缓存只留当前影片这一份（`pluginsUI.marks`），换回之前看过的影片会重算。不放在 `shelfChanged` 里算，因为每次任务报进度都会触发它；也不去 film 块的打开函数里插调用。绑定里只读缓存，从不调用插件，也不在里面算哈希：`CueMarks` 核对缓存的视频和行数与当前相同，再核对这一行的原文和算标注时的原文（`pluginsMarks.originals`）相同，所以字幕换了切片、新标注还在算的时候，原文没变的行照旧显示旧的标注，读音不会消失再出现、行高不会跳；原文改过的行在新标注到之前不显示标注，旧标注不会画在新文字上。
- 开关在 设置 › 字幕外观 最后的「逐词标注」卡片（见 7.4）。
- 手机端（P1.6）：手机不跑插件、不读那份日语大词典，标注由桌面算好经手机同步发过去。协议 v2 新增调用 `films.marks`（`psync.OpFilmMarks`，`packages/sync/marks.go`），参数是 `FilmRef`，回 `FilmMarks{Pending, Lines}`：只列有标注的行，每行是 `LineMarks{Line, Hash, Marks}`，`Line` 是这一行在 `PathSubtitles` 回的字幕里的序号，`Hash` 是算标注时这一行原文的 `psync.LineHash`（FNV-1a 64 位，36 进制），`Marks` 是 `psync.Mark`（字段和 `subtitle.Mark` 完全相同，桌面直接类型转换，tone 用同名常量 `psync.ToneAccent` 等）。手机用 `FilmMarks.Of(originals)` 按序号对齐自己的原文，哈希不同的行（手机上改过原文的行、或手机存的字幕比桌面旧）不画标注。它是 v2 里新增的调用：桌面在 `Hello.Ops` 里列出它，旧桌面回 `unsupported`（v1 桌面回 404），手机照旧不画标注；旧手机不会调它；它只读（不算 `Mutates`），只读设备也能调。所以不升协议版本，`TestFilmMarksIsAnAddition`、`TestDeskAnswersFilmMarks` 和 `synctest` 的 `TestFilmMarksOverV2` 锁住两个方向。
  - 桌面（`state_phonesync_marks.go` 的 `phoneMarks`，计算在插件块的 `state_plugins_phonemarks.go`）：找到手机要的影片（`phoneFilmFind`，任何没上锁的资料库），读手机读到的那份字幕（`phoneSource.Cues`），在引擎 goroutine 上用 `pluginsMarkKeyFor` 算键（和当前影片同一套：视频、说话语言、三个开关、词表和中文词典的内容、是否读大词典、生词本指纹；字幕哈希换成只哈希各行原文的 `annotate.LinesHash`，因为手机的行不带播放器的时间），开关全关时回空列表，手机据此丢掉存着的标注。当前影片的标注（`pluginsUI.marks`）原文和键（除字幕哈希）都相同时直接用；否则查 `pluginsUI.phone.kept`（最近 8 部影片，最新的在后），都没有就在后台 goroutine 里算（和当前影片共用 `pluginsMarkJobFor`、`pluginsMarkJob.run`，每部影片同时只算一份（`running` 按视频），同一个键的调用等同一份结果，键不同的新调用（行多了、或标法变了）先停掉旧的那份，旧的即使后算完也丢掉，不会盖掉新的；开关一改、或 app 退出，正在算的都停掉；算完存进 `kept`。插件目录还没读完（`scanned`）、或读音开着而某个启用的词典还没打开完时（`pluginsMarksReady`），什么也不算，直接回 `Pending`，免得刚启动的桌面回一份没有词表或读音的结果、手机拿它盖掉存着的标注。局域网上的调用最多等 10 秒，经存储中转的调用不等（等着会挡住后面的调用），没算完回 `Pending`，手机过后再问。一行的标注只取决于它的原文和开关，所以同一部影片、同样标法之前算过的原文（当前影片的 `byOriginal`，或 `kept` 里这部影片的那一份）不再重算（`pluginsMarksHad`）：边播边做字幕的影片（#306）每次问都多几行，只算新的行，`kept` 里每部影片、每种标法只留最新一份；手机读到的行和标注用的行都来自 `phoneSource.Cues`（字幕文档的 `View()` 加上用户的更正），所以序号和原文一致，还没做出来的行不在里面，靠每行的哈希保护。生词本只有当前资料库的（`savedBook`），所以虚线只画在当前资料库的影片上，别的资料库的影片只有读音和等级线。
  - 手机：手机没有自己的三个开关，跟桌面 设置 › 字幕外观 › 逐词标注 走；只在 设置 › 字幕样式 加了一个「逐词标注」总开关（`store.SubStyle.NoMarks`，默认开），关掉时字幕层和卡片都不画标注。打开影片时（`session.OpenMarks`）先读影片目录里的 `marks.json`，再在局域网上问桌面（桌面说过不支持 `films.marks` 的不问；`Pending` 时隔一秒再问，最多再问两次），回答写回 `marks.json`，所以离线播放照样有标注；视频下载完时也问一次；播放时 `Follow` 拿到桌面改过的字幕、原文和之前不同时（校对过的、或边播边做字幕多出来的行）再问一次，正在问的那次结束后补问。画法照桌面（`apps/mobile/player/marks.go` 的 `markSpans`）：读音是 `core.Span.Attachment`（上面是半字号的读音，`VerticalAlignTextBottom`），等级是该 tone 的粗实线，已学词是虚线，粗细和位置都按字号的比例（9%、14%）；颜色取主题角色（`Accent`、`Warn`、`Up`、`Muted`），不写字面量。画面上的字幕和竖屏的卡片都画；卡片上没有标注的词照旧是那条淡黄色的「可点」细线。截图场景 `TestScreenshotsPlayerMarks`（`player-marks`，日语原文带读音、两条等级线和一条虚线）。

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
- 防钓鱼：pack 预设的 `baseURL` 和 `keyURL` 只接受 https，只有回环地址（`127.0.0.1`、`::1`、`localhost`）可以用 http。内置服务商的名称加入保留名单（见 6.4）。服务商下拉框里，插件预设排在全部内置预设（含「自定义」）之后，每一行写成「插件 · <包名> · <host>」（`settings.Preset.Plugin` 标出来源包，`settings.Host` 取 host）。NSGo 固定版本的 `ui.Select` 没有带标题的选项分组，所以暂时只靠排在后面和行首的「插件」区分，已提交为 [nsgo-dev/nsgo#906](https://github.com/nsgo-dev/nsgo/issues/906)，代码里留 `// TODO(nsgo#906): …`。点「获取 API Key」时，插件预设先弹一个系统提示框（`base.ShowAlertDialog`），标题写目标 keyURL 的 host，正文写出给出这个地址的包名，用户点「打开」才打开 keyURL；内置预设照旧直接打开。
- 插件预设在启动时晚到：`restoreLLM` → `syncLLM` 在读插件目录之前就按地址认服务商，这时 pack 的地址会被认成 `custom`。所以插件目录读完、以及开关、安装、卸载之后，插件块都会重新认一次服务商（`pluginsFollowLLM`）。规则是不替用户换：当前选的服务只要还在、地址还对得上就保留（`llmProviderFor`），选的是「自定义」就一直是「自定义」，所以 baseURL 和内置预设相同的包不会被换成内置预设，地址恰好和新装的包相同的自定义服务也不会被换成包的预设；只有当前的服务已经不在了，才按地址重认（`settings.ProviderID`）。
- 正在用的包服务消失时（关掉、卸载、换成别的版本、盘没挂上），不能让它的地址和 key 存进 `custom`，否则会盖掉用户自己的自定义服务。做法：开关、卸载、替换和每次读目录之前，先把正在用的包服务按 `pack.<id>` 记下（`pluginsKeepLLM` → `llmRemember`）；消失之后服务显示成「自定义」，但记作 gone（`pluginsUI.gone`，id 和地址），此后 `llmRemember` 和存 settings.json 时插件那块的 `save` 钩子（排在设置那块的 `saveLLM` 之后）都把它算在 `pack.<id>` 名下（`pluginsGoneService`），包回来时它重新显示成包的预设。用户选了别的服务（不是「自定义」）就忘掉 gone（`llmChanged` 钩子）。上次存的服务商若是 `pack.<id>`，启动时就作为 gone 恢复（`restore` 钩子），所以插件目录读完前切换服务也不会写进 `custom`，读完后它显示成包的预设。`llmRemember` 不再直接用 `settings.ProviderID` 的第一个匹配，而是用 `llmProviderFor` 认出当前服务的 id。
- 反馈 issue 的「翻译服务」一行（`feedbackTranslator`）只认内置预设（`settings.Presets`），包服务和其他自定义地址一样写成「自定义」，包名（比如公司内网网关的名字）不会写进公开的 issue。

**E5 翻译风格（P2，P2.3 已实现）**
- 接入位置：设置 › 翻译 的提示词卡片顶部加一行「风格预设」下拉框（`settings.Styles`，读自己的 `settings.StylesApp`，照 `Window.Marks` 的先例由 `settings.Window.Styles` 注入，不扩大 `settings.App`），只在有已启用的 translate-style 包时显示。选项是「不用预设」加上各包的名称；选一个就把包的 `style.prompt` 填进提示词输入框（之后照常可以改），选「不用预设」填回默认提示词。除了提示词本身不另存「选了哪个预设」：下拉框显示的是提示词和哪个包的 prompt 相同（去掉首尾空白比较），改过就显示「不用预设」。
- 清单：`"style": {"prompt": "…{{language}}…"}`，prompt 1 到 4000 个字符，先清洗再计长度。
- 宿主照旧在后面追加 `subtitleReplyRule`（`packages/media/translate.go`），插件改不坏解析；prompt 里没有 `{{language}}` 时照旧在后面写明目标语言（`stylePrompt`）。

**E6 学习任务（P2，P2.1、P2.2 已实现）**
- 五处异步 Chat 流程（`state_explain.go` 的 `explainAsk`、`state_vocab.go` 里 `LookUpWord` 起的 goroutine、`state_recap.go` 的 recap 请求、`palette_app.go` 的面板提问、`state_correct_candidates.go` 的 `correctCandidates`）重复的只有「带超时的 context、起 goroutine、经 `a.update` 回来、按代号丢掉过期回答」这几步，所以只抽出这一段：`ai_ask.go` 的 `aiAsk[T](a, timeout, ask, current, done) context.CancelFunc`。各处的代号、cancel 的存法（explain 是一组 cancel）、回答的解析和显示各不相同，留在原处，没有做成一个更大的 `runTask`，免得为合并而抽象。task 也用它。
- task 数据包：`"task": {"scope": "line"|"word"|"film-so-far", "prompt": "…", "reply": {"fields": [{"key", "label"}], "items": N}, "render": "sections"|"list"|"cards", "maxTokens": N}`。规则：prompt 1 到 4000 字符；fields 1 到 8 个，key 匹配 `^[a-z][A-Za-z0-9_]{0,31}$` 且不重复，label 必填；`items` 只用于 list/cards，不超过 20（默认 8）；`maxTokens` 为 0 或 64 到 4096；为 0 时回答的上限按形状算（`Task.Tokens`：条数 × 字段数 × 每个值 100 个 token，再加 50），算出来超过 4096 的形状 `Validate` 拒收，免得回答总被截断。
- 非界面包 `plugins/task`：`Prompt` 在包的 prompt 后面依次追加回答语言（替换 `{{language}}`，没有就另起一句写明）、「只依据给出的字幕，不要猜测或透露之后的内容」、以及 JSON 回复规则（sections 是一个对象，list/cards 是不超过 items 个对象的数组，每个值都是字符串）；宿主的规则排在最后，包改不掉它们。`Request` 按 scope 只发学习者已经看到的：line 是当前句（选中改错的那句，否则正在播的、刚听过的或下一句，和 I 讲解这句的挑法一样）和它之前 3 句；word 是查词卡上的词、它所在的句子和之前 3 句；film-so-far 是到最远播放点为止的字幕（`lines.Seen`），超过 40000 字符时丢掉最早的。`Parse` 容忍代码块和前后的文字，数组值按行拼接，数字和布尔值转成文字，一个值也没有算 `ErrEmpty`，不是要的 JSON 算 `ErrMalformed`。
- 入口：每个已启用的 task 包在快捷键表里有一行（`pluginsTaskKeys`，`keys.go` 的 `shortcuts()` 加一项，Key 为空），所以出现在命令面板和菜单栏的「学习」菜单里，标签是「插件任务：<包名>」，只在有可运行的对象时出现（影片已打开、字幕已读入；word 还要查词卡开着）。播放器 ⋯ 菜单在自己的项后面加上同样的行（`film.Tasker.TaskMenu`，照 `MoveOpenFilmMenu` 的先例由 film 包自己在 ⋯ 里追加），不往别的块的函数里插调用。task 包的结果没有搬进播放器侧栏的「扩展」标签（E15，第 15 节），仍用自己的侧栏视图。
- 结果显示在侧栏，和「讲解这句」一样占据标签内容的位置（`film.SideTask`、`film/task.go`）：标题是包名，下面是对象（句子、词，或「影片开头到 m:ss」），回答按 render 显示成带标题的分段、列表或卡片，脚注写模型、这次用了多少 token、今天还剩多少。没有 AI 服务（含离线时的云端服务，`paletteTranslator` 的门控）、预算用完、或还没看过任何字幕时，侧栏说明原因，前两种附上打开 AI 服务设置或插件设置的按钮。换影片、或侧栏换成别的内容（比如「讲解这句」或别的标签）时结果关闭，正在等的请求一起停下（插件块自己的 `afterChange` 看 `filmSide`，不往 explain 的函数里插调用；讲解这句被 task 顶掉时仍像被别的标签顶掉一样留在后台，那是 explain 块原有的行为）。line scope 选到的那句还没听到（改错选中了最远播放点之后的句子）时不发送，侧栏说明。模型在上限处停下（`finish_reason` 为 `length`，`media.ErrReplyCutOff`）时说「回答被截断了」，不当作包的回答格式不对。
- 预算：task 包装好后是关的（`plugins.State.EnabledPack` 对没有条目的 task 包返回 false，和 P3a 的脚本插件一样，安装和换新内容时都写成关），用户在插件页打开开关就是同意它花 token，确认框和详情里写明。每日预算存在 state.json 这个包的条目里（`PluginState.Budget`，0 表示默认 50000，可在详情里改，1000 到 1000000），开关、换内容时保留。发送前先估算这次最多用多少（提示词的 `EstimateTokens` 加回答上限）：超过今天剩下的预算就不发送并说明；film-so-far 先把字幕裁到剩下的预算放得下的最近几行（每个字符按一个 token 算，宁多勿少），连最后一行都放不下才拒绝。每次的用量来自 `ChatWith` 的 usage（没有就估算）；请求已经发到服务商但失败了的（重试前的那次、被拒的 key、超时、发出后被取消，靠 `httptrace` 的 `WroteRequest` 判断），每次按提示词的估算计入，`ChatWith` 出错时也带回这部分 usage。这些在 `aiAsk` 之外单独经 `a.update` 计入，所以用户关掉结果、回答被丢掉时 token 照样算；用量按本地日期存在插件目录的 `usage.json`（`plugins.Usage`，app 是唯一的写者，在引擎 goroutine 上写，和查词缓存一样），重启不清零；卸载 task 包时删掉它的那一条。请求头带 `X-Title: Kando plugin <id>`，`max_tokens` 用包的 `maxTokens`。

**E7 导出模板（P2，P2.4 已实现）**
- `saved/targets.go`：`saved.Target{ID, Format, Headers, Columns, NoteType}`，零值是看懂自己的 Anki 笔记（`WriteNotes`，行为不变）；包的模板写 CSV、TSV（`encoding/csv` 的引号规则）或 Anki 文本导入（`#notetype:` 用模板的 `noteType`，默认 Basic）。`saved.Export` 多一个 target 参数，文件名是 `Kando Saved Lines (<id>).csv|tsv|txt`；模板里没有 `{{sound}}` 时不切音频。
- 清单：`"template": {"format": "csv"|"tsv"|"anki", "noteType": "…", "columns": [{"header": "…", "value": "…"}]}`。只能用宿主列出的占位符 `{{original}}`、`{{translation}}`、`{{note}}`、`{{film}}`、`{{time}}`、`{{sound}}`（`plugins.TemplatePlaceholders`，和 `saved.Placeholders` 由根目录测试锁成一致），其余的 `{{…}}` 和没闭合的 `{{`、`}}` 一律拒收；1 到 16 列，值 1 到 500 字符，header 不超过 40 字符且只用于 CSV/TSV，noteType 只用于 Anki。
- 模板有 `{{sound}}` 时，没装 ffmpeg 或部分句子切不出音频，导完的说明和 Anki 导出一样提醒（也给出打开设置的按钮）；没有 `{{sound}}` 的模板不提音频。
- 界面：收藏句子的面板在导出按钮旁边加一个下拉框（「Anki」加上已启用的模板包「<包名> · 插件」），只在有模板包时显示；选中模板时按钮写「导出…」，导完说明写到了哪个文件。选择存在插件那块自己的 `savedPlugins.PluginExportTarget`，包缺席时保留。`state_saved.go` 的 `ExportSaved` 多了一行取 `a.pluginsExportTarget()`，这是导出必经的接点（8.4 已列出），没有别的写法。

**E8 站点包（P4，最敏感）**
- 站点包不往 `kcore.VideoSites` 或 `sites/catalog` 里加任何东西。这两处都是进程级的全局状态（`sites/catalog/catalog.go:35` 的 `sync.OnceValue`，`packages/core/videosites.go:70` 的 `compiledVideoSites`），控制 socket 的浏览器扩展路径也复用同一套判断（`state_browserext.go:194,196`：注释里写着「Any program of the user can call the socket」，下面是 `if !browse.VideoLink(link) || !catalog.Load().On(link)`）。只要往里加，CLI、socket、MCP 的入口就会一起放宽。
- 改为单独做一个 `pluginSites` 匹配器（实现是 `pluginsUI.sites`，每个包一个 `kcore.VideoMatcher`，在 `state_plugins_sites.go`），只给内置浏览器用：工具栏的「下载视频」（`browseNotVideo`、`browseList`、`browseEnqueue`，`state_browse.go`），以及网页里按钮的两个入口 `browsePageAsk` 和 `browsePageAdd`（`state_browse_buttons.go`，经 `browsePageSite` 判断页面的 origin、`browsePageVideo` 判断链接，二者都用同一个匹配器，不信页面发来的任何东西）。网页按钮按页面的网站分开：catalog 网站的页面照旧只能问和加看懂自己网站的视频；插件站点的页面只能问和加同一个包的视频（`browsePageTakes`），所以插件页面加不了 YouTube 这类会借用登录的视频。`browserExtVideo`、remote 的 `browser.state`、`browser.add`、`browser.show` 和 `sites` 都不认插件站点；CLI 和 MCP 经 remote 的 `add` 加进来的插件站点链接，按任何未知网站的链接处理，不算站点包的任务。这些都写了测试锁住（`plugins_test.go` 的 `TestPluginSiteWidensNoGate`、`TestPluginSitePageButtons`）。
- 网页里的按钮（视频页上的「下载视频」和缩略图上的 +）覆盖已同意的插件站点：`browse.ScriptWith(sites)` 把这些网站的规则接在看懂自己的网站后面写进页面脚本（每个网站先过 `kcore.ValidateVideoSite`，不过的不写；JSON 由 `encoding/json` 转义 `<`、`>`、`&`、U+2028、U+2029，写不出 `</script>`）。插件网站没有按钮位置，所以「下载视频」浮在播放器角上，标题取文档标题。页面脚本绑在 `core.WebView.InjectedJavaScriptBeforeContentFunc` 上（`browse/page.go`，读 `BrowseScript`；NSGo 在 [nsgo-dev/nsgo#840](https://github.com/nsgo-dev/nsgo/issues/840) 加的，`nsgo/core/webview.go:26-27`、`:168-175`）。已同意的网站一变（安装、卸载、开关、勾选域名），`browseSitesChanged` 算出新脚本，`browseTakeScript` 把它交给同一个 web view，不重建，后退的历史都在；这次载入的是当前页面的同一个地址，前进的历史还在不在、历史里会不会多出一项，取决于各平台怎么处理同一地址的载入，没有逐平台实测。NSGo 不是等下一次导航才用新脚本，而是脚本一变就立刻用新脚本重新载入 web view 的 source，换掉当前显示的页面，哪怕那是用户自己点过去的页（各平台的 `Configure` 在脚本变了时先换脚本再载入 source：`internal/platform/apple/web.go:373-378`、`:397-398`，`linux/web.go:358-369`、`:398-401`，`windows/web.go:735-762`，Windows 等脚本加好才导航）。所以 `browseTakeScript` 在换脚本的同一次更新里把 source 改成当前显示的页面（用户可能已经从 source 点到了别的页），开始页底下就是空白页，一起交给 web view，只载入一次；正在载入的页被这次载入打断，按「app 打断的载入」处理，不报载入失败（`browseCutShort`）。之前告诉网页按钮的视频状态（`browse.buttons`）一起清空，新页面上的按钮重新询问，和平常换页一样；这样撤回同意或停用包之后，app 不会再把这些视频在资料库里的状态推给那个网站的页面。主窗口显示的是别的页（比如影片列表）时，web view 先不换脚本，等浏览器页再显示时才换（`browseAfterChange` 调 `browseTakeScript`）；设置开在自己的窗口里或盖在主窗口上，不改 `a.page`，所以用户从浏览器页打开设置改同意时，主窗口里的浏览器页仍算在前台，立刻换脚本重新载入。先不换的原因是：后台重新载入的页面不在静音和暂停之列（`browse.QuietScript` 只作用于当时已有的视频），网站自己自动播放就会出声。在这期间页面上还是旧脚本，网页按钮发来的询问和加入照样在 Go 一侧按当前的匹配器判断，所以已撤回的网站问不到也加不了。站点包的清单没有按钮位置（`rows`、`titles`、`cut`）和扩展菜单（`menu`）这些字段，写了就是未知键。
- 在站点包的、已同意的域名上，「下载视频」按包的链接规则判断：匹配的是一个视频，可以加；该域名下的其他页面一律不是视频（按钮变灰，提示和别的列表页一样），也不当成播放列表或频道去列视频（`browseList` 返回 false），所以不会经 `playlistAsk` 用浏览器登录去读列表。没同意的域名、停用的包，和没装包时一样。
- 站点包加的下载（工具栏和网页按钮都一样）一律不带 `--cookies-from-browser`，也不带内置浏览器页面的 cookie。接点：`browseEnqueue` 加入时经 `jobQueue.stamp` 给任务打上 `core.Job.PluginSite`（包 id，存进 library.json）并把 `Cookies` 设为 `core.CookiesNone`；`queueRun` 不给这类任务补设置里的浏览器；`queuePipeline` 只要 `PluginSite` 非空就把 `Tools.Cookies` 和 `Tools.SiteCookies` 都清空，不管 `Cookies` 被谁改过（失败卡片的「借用登录」也一样）。所有用 yt-dlp 的地方（下载、重做、仅字幕影片的串流 `StreamURL`）都经 `queuePipeline` 取 Tools。失败卡片（`FailureRemedy`）对这类任务去掉借用登录、查找浏览器、选择浏览器和完全磁盘访问权限的按钮，说明改成「插件添加的网站上的视频，下载时一律不借用浏览器登录」。
- 前置条件（P4.0）：抽出共用的 `core.ValidateVideoSite` 和 `ValidateVideoSites`，只允许 Go 和 JS 共有的正则子集，不允许嵌套量词、反向引用和环视，长度不超过 256，域名唯一。站点包和 `pluginSites` 用它校验。`catalog.Load` 和 `compiledVideoSites` 不再需要改。实现时比这几条更严，见第 15 节 P4.0。
- 每个域名都要用户单独同意：安装确认框和包的详情里每个域名一个复选框，默认不勾；同意的域名存在 state.json 里这个包的条目上（`plugins.PluginState.Sites`），和 `enabled` 并列，旧文件没有这个字段照样能读，读出来是一个都没同意。不接受 sign-in cookie 配方，不新增 `core.Platform`。
- 站点包的清单：`site.sites` 是 1 到 8 个网站，每个有 `domain`、可选的 `not`（域名下不是视频页的主机，最多 16 个）和 `videos`（1 到 16 个链接规则：`path`，以及成对出现的 `param`、`value`，和 `kcore.VideoShape` 同义）。只有数据：没有脚本、cookie、登录配方、yt-dlp 参数，也没有按钮位置。域名必须是可注册域名或它下面的主机，不能是公共后缀（`co.uk`、`github.io`，用 `golang.org/x/net/publicsuffix` 判断，`apps/kando` 已经依赖 x/net），也不能等于、在下面或在上面包含看懂自己的 `kcore.VideoSites` 或 `sites/catalog` 里任何网站（含是域名的别名）的域名（`ErrSiteKnown`；catalog 不导入任何界面包，`plugins` 可以直接用它）。`pluginSiteOf` 遇到 catalog 认识的链接也一律不算插件站点，作为第二道防线。

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

**E14 候选译文和机器翻译来源（DeepL 等）**（已实现，`feat/plugins-translators`，见第 15 节）
- 贡献块 `translators`（最多 8 项）：每项 `{id, title, languages, local}`，`languages` 是 `"any"` 或 `{from: [...], to: [...]}`（语言标签，规范形式），`local` 默认 false，说自己在本机运行（离线模式也能用），申请了网络或凭据的插件不能写 true。没有 `export` 字段：有翻译来源的插件必须导出 `translate(lines, from, to, context)`，回和 `lines` 等长、同序的字符串数组；可以导出 `candidates(line, from, to, context)`，回字符串数组（改错的候选）。`from` 是影片听出来的语言（whisper 的代码，`media.SpeechLanguageOf`，不知道时为 ""），`to` 是目标语言的标签（`core.TargetTag`，看懂不认识的自定义目标语言原样给出，只交给 `"any"` 的来源）。`context` 是 `{translator}`，候选另有 `current`（这句现在的译文）、`before`、`after`（前后几句的原文）。看懂每次最多给 50 行（`plugins.TranslateBatch`），回答的行数不对、不是字符串算 `bad-reply`；回答里的换行换成空格，每行最多 2000 字。和其他贡献块一样受 `kandoManifest()` 比对（在 `contributes` 里），列在详情页的「添加的功能」里（「翻译来源：<标题>」），算读字幕（`ReadsLines`）。
- 设置 › 翻译 的第一张卡片顶上多一行「翻译来源」（`settings.Sources`，有插件提供翻译来源、或选中的来源不在了时才出现）：「AI 服务」（默认）或已启用插件的各个翻译来源。选了插件的来源时，任务的 `media.Tools.Engine` 是一个 Go 适配器（`state_plugins_translate.go` 的 `pluginsTranslator`），经 runner 调插件，受插件自己的看门狗、预算和熔断约束，插件等宿主（`credentialFetch`、`llm.chat`）时照旧 `PauseFor`；`EngineName` 是来源的标题（界面语言）。选择存在 settings.json 翻译那一部分的 `translateSource`（`{plugin, translator, title}`）。选中的来源有五种状态（`pluginsSourceState`）：没选；待定（启动时插件目录还没读，或插件已打开、还在验签或载入）；在运行；坏了（插件已打开并提供这个来源，但跑不起来：验签没过、被看懂停用、吊销列表过期而暂停、有凭据而系统没有安全存储，即 `pluginsRunnable` 为假）；不在了（插件被关闭、卸载或不再提供这个来源）。前三种都用插件翻译：用哪个插件在每次翻译开始时才定（`pluginsTranslator.ready`），所以启动时恢复的任务会等插件载入（最多 60 s），不会悄悄改用 AI 服务；等的时候任务被停下就直接返回，不调插件。坏了的来源让翻译按 `not-running` 失败，侧栏状态行说「翻译来源「X」没有在运行」并指向插件页。只有不在了才改用 AI 服务，设置页那一行写「翻译来源「X」已不可用……改用 AI 服务翻译」，消息栏每次运行说一次；插件重新打开后又用回它。状态一变（选了来源、插件载入或关闭），侧栏状态行重新检查，资料库重新提议翻译没翻译的影片（`pluginsTranslateFollow`，在插件块自己的 afterChange 里）。离线模式下问的是选中来源的 `local`，不是 AI 服务：云端来源被拒（状态行和卡片说「翻译来源「X」不在这台电脑上，而离线模式开着」，指向隐私页），本机来源照常；选来源时把 `local` 一起存下，插件目录读到之前用它。来源翻译时，whisper 的语言代码先换成翻译来源用的标签（`plugins.SpokenTag`：`no` → `nb`，`jw` → `jv`）。选了插件来源而 AI 服务没配好时，播放器进度徽章不显示「终校」一行，和任务跳过终校一致。
- 离线模式拒绝了不在本机的来源时（`media.ErrOffline`），卡片说离线模式开着、指向隐私页，而不是照抄错误文字。
- 插件的失败带码：插件以 `{code, message}` reject，或在同步导出里 throw（v0.37.0 及更早的看懂只认 async 导出的拒绝，同步 throw 会丢掉 code，nsgo#847；要支持这些版本的插件仍写 async 导出），看懂认得 `quota`、`auth`、`no-credential`、`language`、`rate-limit`、`unavailable`，自己再给 `not-running`、`bad-reply`（`plugins.TranslateError`，文字是「<code>: <插件说的话>」，library.json 存的就是它，读回来照样认得）。失败卡片（`jobfailure.SourceHint`）对这几种码各有一句以来源名开头的话，`auth`、`no-credential`、`not-running` 带「打开设置」到插件页，`language` 到翻译页；其他码和没有码的失败照旧「翻译来源「X」失败：…」。
- 官方 DeepL 插件 `kando.deepl` 的源码在 `apps/kando/plugins/official/deepl/`（`kando-plugin.json`、`plugin.tsx`、给 `nsgo publish` 读的 `nsgo.yaml`，`kandoManifest()` 直接 import 这份清单），和 P3c 的 AnkiConnect 一样由 `make official-plugins` 签名、`official.Wrap` 打成 `.kandoplugin`，从 kando-plugins 的 `scripts/kando.deepl/` 发布。它只签 `kando.net.credentialFetch` 一项权限，不签任何网络规则：DeepL 的密钥和请求都由看懂按清单里的凭据代发。
- 底座已做（`refactor/media-stage-error`，见第 15 节）：`media.Tools` 有可选字段 `Engine media.SegmentTranslator` 和 `EngineName`。设了 `Engine`，`Tools.Translate` 就委托过去，`Tools.CheckTranslate` 只查它（它实现 `TranslateChecker` 时问它自己，不再要求 AI 服务的模型和登录）；没设就走 `ChatTranslator`，和以前完全一样。离线模式下，`Engine` 不说自己在本机（`Local() bool`，`ChatTranslator` 也有这个方法）就在发送前返回 `media.ErrOffline`（算 `Engine` 的失败，卡片会说出来源名）。校对、终校和术语建议仍走 `Translator`，`Engine` 只管翻译；设了 `Engine` 而 `Translator` 没配好（没有模型、地址、key 或登录，或离线模式下不在本机，`Tools.chats`，不联网判断）时，`Make` 不做这三步，而不是在它们上面失败。`EngineName` 留空是调用方的错，失败时用 engine 的 Go 类型名（例如 `*plugins.translator`）代替，不会被当成 AI 服务的失败。没有改动任何一处 `media.Tools` 字面量或断言。
- 改错的候选：`Tools.Candidates(ctx, req)` 在 `Engine` 也有 `Candidates(ctx, media.CandidateRequest) ([]string, error)` 方法时问它（接口 `candidater` 不导出，插件那边的类型实现同名方法即可），结果和 `ChatTranslator.Candidates` 一样过滤（去掉当前译文、空白和重复，最多 `CandidatesMax` 条）；没有这个方法就交给 `ChatTranslator`。改错流程（`state_correct_candidates.go`）调 `Tools.Candidates`：选中的插件来源在运行、并导出了 `candidates` 时问它（这时不需要 AI 服务），否则问 AI 服务；来源的失败用和卡片一样的话说出来，不给「打开 AI 服务设置」。`kando.candidates.offer` 不再需要，由 `candidates` 导出代替。
- 插件带凭据访问网络必须走 `kando.net.credentialFetch`，key 不进 JS（见第 6 节）。
- 插件导致的失败要能识别出来：`media.Make` 的每个失败都是 `*media.StageError{Stage, Source, Err}`，`Stage` 是 `check`、`download`、`transcribe`、`translate`、`export` 之一（`media.StageCheck` 等常量；校对和终校的失败算 `translate`），`Error()` 仍是「步骤名: 原错误」，`Unwrap` 让 `errors.Is`/`errors.As` 照常工作。只有完全是 `Engine` 自己的失败才带着名字进 `Source`（`media.EngineOf`）：检查时把看懂自己缺的工具和来源的抱怨 `errors.Join` 在一起的，不算来源的失败。`StageError` 的 `Err` 为 nil 时 `Error()` 只是步骤名，`Detail()` 给不带步骤名的错误文字。app 里原来按 `": "` 切错误文字的地方都改成 `errors.As`，并从 package main 挪进了非界面包 `apps/kando/jobfailure`：`jobfailure.Stage`（原 `failedStage`）、`jobfailure.Hint`（原 `failureHint`，参数改成状态行和 `*media.StageError`，不再依赖 `films.Film`）、`jobfailure.TranslationHint`（原 `translationHint`）、`jobfailure.Of`、`jobfailure.ToolMessage`（原 `toolMessage`，订阅和播放列表也用它）；`phonesync/jobs.go` 的 `failedStep` 也改成 `errors.As`。`films.Film.Failure` 带着它给卡片用。`Source` 不为空的失败卡片先于其他已知原因写「翻译来源「X」失败：…」（`precheckFailedSource`），只看来源自己的错误，不给「打开设置」，因为来源的设置不在 AI 服务那一节。
- library.json：`error` 照旧存完整文字（以步骤名开头，旧版 Kando 照旧能切出步骤），另加 `errorStage`、`errorSource` 两个可选键，只有 `job.Err` 本身就是 `*media.StageError` 时才写。读的时候有 `errorStage` 就按它还原；没有（旧文件）就用 `media.ReadStageError` 从文字里认出这五个步骤名，和以前的读法一致。完成但没翻译的影片：`core.Job.TranslationSource` 记下让它没翻成的来源（`Make` 用 `EngineOf` 从检查或翻译的失败里取），library.json 加可选键 `translationSource`，旧文件和旧版读到都当作 AI 服务的；`films.Film.TranslationSource` 带给 `jobfailure.TranslationHint`，卡片同样写「翻译来源「X」失败：…」。

**E15 插件 UI**（已实现，出入见第 15 节）
- 播放器侧栏只加一个统一的「扩展」标签，只在有正在运行的脚本插件贡献了面板时出现。原来的 `var panelTabs` 改成 film 的 `screen.panelTabs`，按 `film.Extender` 读出有没有面板；「扩展」是一个只有拼图图标的标签，带读屏名称和悬停提示。标签内只有一个面板时是标题，多于一个时用 `ui.Select` 切换，选择存在 settings.json 的 `pluginPanel`（`plugin:<插件 id>:<面板 id>`，插件不在运行时保留）；标题下写「来自插件「<插件名>」」。正文是插件自己的视图（`tsx.View`，在 `script.Runner.Panel` 里外包 `core.Catch`）。
- 贡献块 `contributes.panels`：最多 8 项，每项 `id`、`export`（插件导出的 TSX 组件）、`title`，可选 `icon`（只能是 `plugins.PanelIcons` 里的 Lucide 名，看懂自己画，不收插件的图片）；`Probe` 检查导出存在，`kandoManifest()` 的比对覆盖它，详情页「添加的功能」列出「播放器「扩展」标签里的面板：<标题>」。
- 面板的视图是同一个包在另一个运行时里再跑一次入口模块：沙箱和插件函数的完全一样（`script.Sandbox`：`Limits`、从零建的 Grant、同一个 Storage 分区），Grant 用插件载入时的那份；Host 是这个视图自己的（`core.Host` 同一时刻只服务一个 app），每次构建（包括「重试」）都新建一个，handler 和插件的一样，`pluginsGuard` 只在它是当前显示的视图时认它，配额和插件共用；视图处在出错状态时没有 Host 是它的，事件不发给它。宿主参数 `host.params` 写着 `panel` 和 `export`，入口模块据此自己 `mount`：`if (host.params.panel === "deck") mount(Deck)`；插件为函数载入或预读时参数是空的，什么都不挂。
- 面板的视图抛错（载入、渲染、之后处理事件时）、某一段超过 `Limits` 的预算或超了配额，`core.Catch` 在标签里显示「插件「X」的面板出错了」和「重试」，播放器不受影响；每次失败都像一次失败的调用一样计入插件的熔断，没出错就被撤下的视图算一次成功，最近 50 次里 3 次失败就停用插件并记进 health.json。视图开着时有自己的看门狗（`script.PanelWatch`）：引擎连续 3 个 5 s 窗口忙于 60% 以上，或活堆比视图建好时多出 512 MiB，就先把视图撤下 2 s；撤下后引擎闲了（忙不到 30%）或堆回落，才按 `slow`、`memory` 停用插件，否则重建视图，连续 3 次冤枉后不再怀疑它。每次构建写 in-flight 标记（在单独的 goroutine 上写，不在建树时读写磁盘；这个插件正有调用在跑时不写，免得清掉调用的标记）。
- 正文只在换了面板或插件重新载入时重建（`Extension.Run`），播放、换句、别的状态变化都不重建，面板里的焦点和滚动位置保住。
- 插件设置页只有声明式 schema（见 7.3），没有 `tsx.View` 的退路：清单里没有设置页视图的字段，`tsx.View` 只用在播放器「扩展」标签的面板上。

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
- 不引入 NSGo 之外的新代码来源。goja `v0.0.0-20261027234916-5f913d1693d9` 已经以 indirect 形式出现在 `apps/kando/go.mod:46`；P3.0 实测链接 tsx 之后 `go mod tidy` 会在 `apps/kando/go.mod` 多两行 indirect（`github.com/dlclark/regexp2/v2 v2.11.0`、`github.com/google/pprof v0.0.0-20230207041349-798e818bf904`），版本都是 NSGo 的 go.mod 已经选定、go.sum 里已有的。P3a 起 regexp2 是直接依赖（第 11 节）。代价是二进制大 19–21 MiB（约 +37%），见第 15 节的 P3.0 测量。

### 3.3 已核实的运行时语义

- **线程**：`tsx.Load(cx *core.Scope, ...)`（`tsx/view.go:65`）需要一个活着的 Scope，并且在 `Limits.Load` 之内同步执行入口模块。`Plugin.Call`（`view.go:118`）的注释写着 "Call it on the engine's goroutine"，签名是 `Call(name string, args []any, done func(result any, err error))`。`pkg.Trust.Verify` 在引擎 goroutine 之外做。所以 `ready` 先在后台 goroutine 里做 Scan 和 Verify，再按每个 `a.update` 只载一个插件的节奏逐个 Load，每次 Load 的预算是 1s。这样 UI 最长只卡 1s，不会是 12 个插件 × 3s。P3.0 实测正常的包远低于这个上限：空包 Load p50 0.6–0.8 ms，1.15 MB 源码的包 p50 30–47 ms（第 15 节）。
- **预算只算同步片段**：`tsx/sandbox.go:311` 的 `within` 只在最外层同步 JS 片段上挂 `Limits.Call` 计时器：`r.depth > 0` 时不计时，`fn()` 返回就停表，而且实现方式是 `time.AfterFunc(budget, func() { r.vm.Interrupt(ErrBudget) })`（`:322`）。所以 `await host.call(...)` 或 `await fetch` 的等待时间不算进预算。Kando 把每个同步片段的预算设为 500ms，Load 设为 1s：网络型插件不会被误杀，CPU 重的插件必须自己用 async 分片，否则会被打断。默认的 2s，以及 user-value 方案的 5s，都会让 UI 整段卡住，不采用。P3.0 实测这条语义有两个后果：ErrBudget 只让这一次调用失败，不停掉插件（`within` 在 `:322-326` 只 Interrupt 再 ClearInterrupt，不调 `r.stop`），模块级变量里攒的数据一直留到 Close；每个片段都不到 500ms、用 `await new Promise(r => setTimeout(r, 0))` 让出的调用永远不会被切断，可以让引擎几乎一直忙（输入事件等待 p50 449ms）、内存一直涨。所以 Kando 在 Limits 之外还要有自己的看门狗，在片段之间 `Close` 插件（6.6、第 15 节）。
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
- **host.call 超时**：`core/host.go:60` 的 `HostCallTimeout = 30 * time.Second`，JS 不传 `{timeout}` 就用它。Kando 的 Chat 客户端默认两分钟并且会重试，慢的 LLM 回复会在插件这边先超时。所以有三条规定：插件调 `kando.llm.chat`、`kando.sound.read` 和 `kando.net.credentialFetch` 这三个要等宿主之外的事的 handler 时必须传 `{timeout: 150000}`（写在 5.3 给插件作者的规则里，`kando-plugin preview` 对没传的调用写一条提醒）；宿主侧 handler 自己在 120s 时取消 context（`pluginsCallTimeout`），以 `timeout` 拒绝；`llm.chat` 上的 `ErrHostTimeout` 不计入熔断。

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
  - yomitan-zip 内部只检查不解压：条目名可以带子路径，但不能有 `..`、绝对路径或反斜杠，不能是符号链接，不能再套 zip。条目数（目录条目也算）和 manifest、外层文件共用 64 的上限。拿真实词典量过（第 15 节「P1.3 真实词典的测量结果」）：JMdict 英文版 55 个条目、Yomitan 版 CC-CEDICT 22 个，装得下；Jitendex 有 475 个条目（219 个 term bank 加上 251 个 SVG 图），整包 477 个，被拒收（`ErrTooLarge`）。上限改成多少由 owner 定（12.3 第 8 条）。里面的文件校验和不符或解压出错，算 `ErrBadArchive`。
  - `archive/zip` 打开 zip 时会先把中央目录的每一条记录都读进内存，不管记录数是多少，所以一个由几百万条空记录组成的 zip 会在任何上限生效之前就分配几 GB。外层 zip 和嵌套 zip 都经 `openZip` 打开：它在 `zip.NewReader` 读目录期间最多放行 1 MiB 的读取，超出就是 `ErrTooLarge`；64 个条目的目录加上最长 64 KiB 的注释远小于这个数。
- **从文件夹安装**：用 `Lstat` 逐项检查，遇到符号链接或特殊文件就拒绝，再把普通文件复制进 staging，最后校验的是 staging 里的副本。不能先校验原目录再复制，那样有 TOCTOU 问题。打开每个文件后再用 `os.SameFile` 比对 Lstat 的结果，检查和打开之间被换成链接的文件也会被拒绝。文件管理器写进文件夹的文件（访达的 `.DS_Store`，资源管理器的 `Thumbs.db`、`desktop.ini`，后两个不分大小写）在复制时跳过，`Validate` 列文件时也跳过：不检查、不计数、不算进哈希，也从不读取。所以用户在访达里打开已安装包的目录后，下次 `Scan` 这个包照样有效、哈希不变。zip 里的这些文件仍按普通文件名规则处理（`.DS_Store` 以点开头，被拒收）。
- **内容哈希**：对排序后的「路径、长度、字节」序列做 SHA-256，授权绑定在这个哈希上。路径和内容的长度都写成 8 字节大端整数，字节在名字和内容之间、或在相邻文件之间挪动都会改变哈希。
- **字段规则**（`plugins.Validate`）：`version` 由字母、数字和 `.`、`+`、`-` 组成，不超过 32 字节；`license` 必填，不超过 64 个字符；`author` 可选，不超过 80 个字符；`homepage` 可选，规则同 https 地址；`Text` 的键必须是规范写法的语言标签（`zh-Hans`，不是 `zh-hans`），这样 `Text.In` 能用 app 的语言直接找到；名称和 label 不超过 40 个字符，`description` 和 `attribution` 不超过 500 个字符，都先清洗再计长度。glossary 的 `from`、`to` 是语言标签，1 到 5000 条，每条原文和译文各不超过 200 个字符；ai-service 的 `group` 只能是 `cloud` 或 `local`，`model` 不超过 200 个字符，地址不能带用户名和密码；dictionary 的 `format` 是 `yomitan-zip`、`cc-cedict` 或 `tsv`，只有 yomitan-zip 的文件以 `.zip` 结尾（yomitan-zip 是 Yomitan 导出的词典 zip，只读其中的 `term_bank_<n>.json`；cc-cedict 是 CC-CEDICT 的 UTF-8 文本；tsv 每行是 `词条<TAB>读音<TAB>释义` 或 `词条<TAB>释义`，`#` 开头的行是注释，第三列之后的列忽略）；wordlist 有 1 到 32 个 tier，tier 的 id 匹配 `^[a-z0-9][a-z0-9-]{0,31}$` 且不重复，`tone` 是 `accent`、`warn`、`success`、`muted` 之一；它的文件每行是 `词<TAB>tier 的 id`，之后的列忽略，`#` 开头的行是注释，没有 tab 的行和 tier 没声明过的行忽略，同一个词再出现时保留第一次的 tier，词按小写比较，一个词也没留下的词表打开时以 `wordlist.ErrEmpty` 拒绝（P1.4）。task、translate-style、export-template 的字段规则（P2）写在 E6、E5、E7。清单里只能有自己那个 kind 的字段（task、style、template 三个键分别对应三种 kind）。

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

`capabilities` 用 `pkg.Capabilities` 的 JSON 形状，只允许出现 `host` 和 `network` 两个键，`network` 只允许 `http` 规则。每条规则都必须写 `methods`，并写 `paths` 或 `pathPrefixes`，`host` 是一个确定的主机名（不带 `*`），最多 16 条；`network.hosts`（放行任意请求）一律拒收。下面的例子是完整形态，P3b 起全部认得（第 15 节）。贡献块 `lookups`、`subtitleFormats`、`exporters`、`annotators` 各最多 8 项，每项有 `id`、`export`（插件导出的函数名）、`label`；`subtitleFormats` 另有 `extension`（不带点，只能是 `plugins.SubtitleExtensions` 里的字幕和文本扩展名：srt、vtt、ass、ssa、sub、lrc、txt、tsv、csv、json、md、ttml、xml、sbv、smi，文件写在视频旁边，不能是 bat、exe、js 这类可执行的扩展名），`exporters` 另有 `takes`（`lines`、`words` 之一或两个），`lookups` 和 `annotators` 可以有 `languages`。`panels`（E15）最多 8 项，每项是 `id`、`export`（TSX 组件）、`title` 和可选的 `icon`（`plugins.PanelIcons` 之一）。`translators` 最多 8 项，每项 `{id, title, languages, local}`，没有 `export`（固定调 `translate`，可选 `candidates`），见 E14。`credentials` 最多 4 项，`header` 不能是 Host、Cookie、Content-Type 等由请求本身或看懂设置的头，`format` 恰好含一个 `{secret}`，`hosts` 是 1–8 个可注册域名下的主机名（不是 IP、不带 `*`、不是公共后缀），`methods` 只能是 GET、POST、PUT、PATCH、DELETE，`pathPrefixes` 1–8 个、以 `/` 开头、不含 `..`；声明了 `credentials` 就必须申请 `kando.net.credentialFetch`，反之亦然。

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
    "host": ["kando.saved.read", "kando.words.read"],
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

上面是展示完整形态的示例。实际发布的官方插件在 `apps/kando/plugins/official/`（`ankiconnect/`、`deepl/`），AnkiConnect 只有导出目标，申请得更少（没有命令和事件，`optional` 只有 `kando.sound.read`），见第 15 节 P3c。

带凭据的插件（官方的 DeepL 翻译来源，`official/deepl/kando-plugin.json`）换掉的是下面这几个键，语言表只列了几个。它只申请 `kando.net.credentialFetch`：要翻的句子由宿主作为 `translate` 的参数交给它，不需要 `kando.lines.seen`；候选译文由 `candidates` 导出提供，没有 `kando.candidates.offer` 这个权限（5.3）。`methods` 和 `pathPrefixes` 是必填项，宿主据此限制 `credentialFetch`：

```json
"capabilities": {"host": ["kando.net.credentialFetch"]},
"contributes": {"translators": [{"id": "deepl", "title": {"en": "DeepL"}, "languages": {"from": ["en", "ja", "zh"], "to": ["en", "ja", "zh-Hans", "zh-Hant"]}}]},
"credentials": [{"id": "deepl", "title": {"en": "DeepL API key", "zh-Hans": "DeepL API 密钥"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["api-free.deepl.com", "api.deepl.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate", "/v2/usage"]}]
```

这一节的 JSON 例子由 `plugins` 的 `TestDocManifestExamples` 逐个过 `Parse` 和 `Validate` 的字段检查（片段换进它前面那份清单，脚本包的 `sha256` 补上），例子和规则对不上时测试失败。

### 4.3 让贡献声明也受签名保护

`kando-plugin.json` 在签名之外，所以清单里的 `capabilities`、`optional`、`contributes`、`credentials`、`settings`、`events`（`signature.SignedKeys`）必须由签名覆盖的东西再说一遍，宿主逐键比对（`signature.CheckManifest`，这六个键以外的键不能有值），任何不一致都拒装，提示「这个文件被改动过」。签名覆盖的说法有两种来源：

- **包签了清单**：`pkg.Contents.Extra`（`tsx/pkg/package.go:38`，[nsgo-dev/nsgo#761](https://github.com/nsgo-dev/nsgo/issues/761) 在 `f377daba0a2a` 已修）是 app 自己签进包里的元数据，验签后原样出现在 `Info.Extra`，改一个字节签名就验不过。看懂把上面几个键写成一个 JSON 对象签进去（`signature.SignedManifest`），`signature.Verify` 验签之后直接和清单比，不跑插件的代码；`script.Probe` 对这种包什么也不做，贡献点要的导出函数改在 `script.Load`（用户同意之后）检查，缺了就按 `ErrTampered` 载入失败。
- **包没签清单**：已经发布的包（`kando.ankiconnect`、`kando.deepl` 1.0.0）和 `nsgo publish` 签的所有包都是这种。TSX 包必须导出 `kandoManifest()`，返回的对象要和清单的这几个键完全一致。`kandoManifest` 必须是普通函数，不能是 async：预读只收同步返回的对象，返回 promise 的以 `ErrNoManifest` 拒装。

没签清单的包，宿主验签之后在一个零授权的预读沙箱里调用 `kandoManifest()`。注意：插件的模块顶层代码在用户同意之前就会在这里执行，这正是沙箱必须收紧的原因：

```go
l := tsx.DefaultLimits // keeps Harden, Stack, Body
l.Load, l.Call = time.Second, 250*time.Millisecond
l.Nodes, l.Timers, l.Requests, l.Storage = 1, 1, 1, 4<<10
probe := "pluginprobe." + randomHex(8) // throwaway partition, deleted right after
s := tsx.Sandbox{Grant: &pkg.Capabilities{}, Host: nil, Limits: &l, Storage: probe}
```

`Call("kandoManifest")` 一返回就 `Close()`，然后删掉 `probe` 分区。如果不设 `Storage`，`Sandbox.of` 会用 `App.ID` 作分区名（`view.go:37-39`），插件就能在安装前往一个卸载时不会被清理的分区里写数据。

然后把返回值和清单逐键比对（同一个 `signature.CheckManifest`）。此外还要满足：

- 包的 `Capabilities.Host` 必须等于清单 `capabilities.host` 加上 `optional`，包的 `Capabilities.Network` 必须等于清单的 `capabilities.network`，其他能力字段必须全部为零值。
- 包的 `App.ID` 必须是 `ai.yldm.kando.plugin.<id>`，并且被签名证书的 `Apps` 模式覆盖（`tsx/pkg/certificate.go:121-125`）。
- 清单声明的每个导出函数都必须出现在 `Plugin.Exports()` 里。
- 用到的 host handler 和声明的是否一致，宿主不检查，也不因此拒装：代码调哪些 handler 不能静态确定，没声明的调用本来就被 Grant 和 handler 的 `Effective` 检查以 `denied` 挡住。目录的 CI 工具 `kando-plugin preview` 读签名包里的代码文本，在 PR 评论里给审核者写提醒（不让包失败）：代码里 `host.call("kando.…")` 写出了名字、清单却没声明的权限，声明了、代码里却从没这样调过的权限，以及调 5.3 那三个要等的 handler 却没传选项（`timeout`）的地方。声明了 `line.changed` 事件的插件不会因为没调 `kando.line.current` 被提醒：这个事件只在它有这项授权时才推。经变量或运行时拼出来的名字看不到，所以只是提醒。它最多读 1000 个调用、每个调用的参数最多读 64 KiB、每行最多列 16 个名字，超过 1000 个调用时写明只读了前面的，并且不再说哪些权限没调过，所以包撑不长评论，也拖不慢 CI（`hostCallsMax`、`hostCallArgsMax`、`noteNamesMax`）。

兼容：两种包看懂都认，已经发布、已经安装的包不受影响。

- 没签清单的包走的还是原来的 `kandoManifest()` 比对，验签和比对的结果和以前一样；线上 `kando.ankiconnect`、`kando.deepl` 1.0.0 的两个文件只有 `bundle.js` 一个条目，用新代码按官方根验签、预读都通过。
- 签了清单的包，看懂 0.37.0 及以前（NSGo `ec272782be28`）不认 `extra` 这种文件，把它列进 `Info.Ignored`，`checkSigned` 因此按 `ErrBadCapabilities` 拒装，提示的还是「权限不对」。0.37.0 之后的每个版本都从已经固定 NSGo `729f238f5678` 或更新版本的 main 发布，认得这个条目，所以只有清单的 `kando` 下限不低于 `signature.ManifestEntrySince`（0.38.0）的插件才能签它（`signature.SignsManifest`）；`official.Wrap` 拒绝不满足这一条的包（`ErrSignedManifest`），也拒绝签的内容和清单不一致的包（`ErrMismatch`）。
- 认得这个条目不等于拿它来比对：在本改动之前发布的版本（main 上的 0.38.0 很可能就是，release 工作流在 main 每次 CI 通过后自动发版）认得条目，不拒装，但不比对它，仍在预读沙箱里调用 `kandoManifest()`。所以签了清单的插件在它的 `kando` 下限达到第一个比对签名清单的版本之前，仍要导出 `kandoManifest()`；那个版本要等本改动随某个版本发布后才知道（对本改动的提交跑 `git tag --contains`），接上签发端时再定。
- 现在还没有签发方会写这个条目：`nsgo publish` 没有参数能把 `Contents.Extra` 签进去（`cmd/nsgo/publish.go:130-137` 只填 `App`、`Source`、`MinRuntime` 和 `nsgo.yaml` 的能力，没有 `Extra`），官方插件和第三方插件都由它签名，所以 `official-plugins.sh` 照旧，插件仍要导出 `kandoManifest()`。`script.PackFolder` 也不签：开发者包要和它发布后一样被检查，否则漏写 `kandoManifest()` 的插件在开发时发现不了。两处都留了 `TODO(nsgo#924)`（[nsgo-dev/nsgo#924](https://github.com/nsgo-dev/nsgo/issues/924)）：等 `nsgo publish` 能签以后，对 `SignsManifest` 接受的清单签上 `SignedManifest`，并按上一条继续要求 `kandoManifest()`，直到清单的下限达到第一个比对签名清单的版本。

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

// P2 (as implemented) added Task *Task `json:"task"`, Style *Style `json:"style"` and Template *Template `json:"template"`, and PluginState.Budget; P4 adds Site.

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
// on PluginState since format 1 (P4.1), not part of Grant: a site pack's consented domains are not intersected by Effective
	Sites   []string `json:"sites,omitempty"`

// Grant is what the user agreed to, in full: an update keeps it only if what it now declares is a subset of each part.
type Grant struct {
	Perms       []Perm         `json:"perms,omitempty"`
	Network     []pkg.HTTPRule `json:"network,omitempty"`
	Credentials []Credential   `json:"credentials,omitempty"` // id, hosts, header, format, methods, paths as granted
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

in-flight 标记从 `Call` 开始一直保留到 `done` 回调，所以 await 之后恢复执行的那些同步片段也在覆盖范围内。每次调用都要写一次小文件，P3.0 实测写临时文件、rename、删除不 fsync 时 p99 0.13 ms（Ubuntu ext4）到 0.39 ms（Mac Studio APFS）；加 fsync 后 Mac 上 p50 3.3 ms、最大 70 ms（Go 在 darwin 上用 `F_FULLFSYNC`），所以不 fsync：进程卡死被强退或崩溃时内核照样会写下去，只有断电会丢，丢了只是少停用一次。每次调用的数据量也有上限，例如一批不超过 50 行，失败就丢弃这次的结果。

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

`kando.candidates.offer` 没有实现（第 15 节 P3b）：改错的候选由翻译来源的 `candidates` 导出提供（E14），不需要这个权限。申请它的插件照旧以 `ErrUnknownPerm` 拒收。

`kando.glossary.suggest`（第 15 节「术语建议」）：`{film, terms: [{source, target}]}`，`film` 是这个插件知道的影片 id（`film.opened` 事件和 `kando.film.current` 给的那个），可以不写，写了就必须是正在打开的影片，否则回 `not-found`；没有打开的影片也回 `not-found`；打开的影片没有视频，或术语表还没跟上刚打开的影片（`a.gloss.video` 和影片的视频不同）时回 `unavailable`。影片自己的术语和建议文件还没读完时照收（插件听到 `film.opened` 就调用时多半还没读完），读完后术语表已有的词不列出。一次最多 `plugins.SuggestMax`（20）条，原文和译文清洗后（去掉看不见的字符和控制字符、合并空白，`plugins.Suggested`）各 1–`plugins.SuggestLongest`（60）字，和 AI 服务自己的术语建议同一个上限；超出、缺字段或类型不对时整次以 `bad-request` 拒绝。插件提议的术语不进术语表，只在内存里接在 AI 服务的建议后面（`a.gloss.suggest.plugin`），列在播放器「术语」标签的「片中找到的术语」里，下面一行写「来自插件「<插件名>」」（`glossarySuggestFromPlugin`）；换一部影片就清空，一部影片打开期间所有插件的提议合计最多 `plugins.SuggestHeld`（60）条。术语表（这部影片自己的、系列的、全局的和术语包的，锁定与否）已有的词、AI 服务已经建议的词、用户已经「不用」的词、同一插件或别的插件已经提议的词都不收，所以插件永远改不了已有的条目，也不会顶掉锁定的条目。回 `{added}`，即这次新列出的条数。只有用户点「添加」时才写进术语表，存成这部影片自己的、`OriginAuto`、不锁定的条目，和 AI 服务的建议同一步（`AcceptSuggestion`，撤销后重新出现在建议里）；点「不用」时写进影片旁建议文件的 `dismissed`（`media.DismissSuggestion`），之后 AI 服务和插件都不会再建议它。

P3b 实现的几个 handler 的形状（都在后台跑，等待期间暂停这次调用的看门狗，失败只回错误码）：

- `kando.llm.chat`：`{system, user, maxTokens}`（system 最多 8000 字、user 最多 32000 字，maxTokens 默认 1024、最多 4096），回 `{text, model, tokens}`。预算是插件自己的每日 token 数（`State.Budget`，默认 50k，详情页可改），按「估算的 prompt + maxTokens」先查，超出回 `budget`；没有服务、离线用云服务、隐私设置不让用时回 `unavailable`；回复被截断 `cut-off`，宿主 120 s 取消 `timeout`。实际用量（服务给的或估算的）不论成败都计入 `usage.json` 并写进 audit.log。同一插件同一时刻只有一个请求，第二个回 `busy`。
- `kando.sound.read`：`{saved: <收藏句子的 id>}`，回 `{mime: "audio/mp4", base64, durationMs}`，和导出到 Anki 一样用 ffmpeg 切（`saved.SoundFor`），最多 2 MiB；没有 ffmpeg 回 `no-ffmpeg`。也接受 `{current: true}`（正在播放的这一句）。用户在收藏句子面板按下的导出（`exportNotes`）进行期间，这个插件读声音按这次导出计：最多读「交给它的句子数加 8」次，不计入每分钟 60 次的限额（`pluginsAllowSounds`）；调用一结束额度就作废，剩下的不留给以后，导出之外（命令、定时器）的读声音照旧计入限额。额度属于正在导出的插件，导出期间它自己的定时器也能用，但用不过这个数（P3c）。
- `kando.net.credentialFetch`：`{credential, method, url, header, body}`，见 6.3。回 `{status, header: {"Content-Type"}, body}`，body 最多 4 MiB，请求体最多 1 MiB，插件自己的头最多 16 个。没存密钥回 `no-credential`，系统没有安全存储回 `unavailable`。

贡献点的函数收到的参数（宿主调用，不需要权限，插件没在运行时不调）：

- `lookUp(q)`：`q = {word, base, reading, line, spoken, language}`，回 `{word, reading, meaning}` 或 `null`；没有 `meaning` 算没查到。等 15 s，超时问下一个来源。
- `exportSubtitles(formatID, cues)`：`cues` 是 `Export.Cues` 的结果，每条 `{startMs, endMs, lines: [{text, original, dim}]}`，用户选的内容、顺序、缺译处理和延迟都已套用；回字符串（最多 8 MiB），宿主写文件。
- `exportNotes(items, settings)`：`items` 是收藏句子和生词，形状和 `kando.saved.read`、`kando.words.read` 一样，多一个 `kind`（`line` 或 `word`），各最多 500 条；`settings` 是插件自己的设置值；回 `{sent, skipped}`。
- `translate(lines, from, to, context)` 和 `candidates(line, from, to, context)`：见 E14。
- `annotate(lang, lines)`：每批最多 50 行原文，回和 `lines` 等长的数组，每项是这一行的标注 `[{start, end, ruby}]` 或 `[{start, end, tone, underline}]`，偏移按 JavaScript 的 UTF-16 码元算（宿主换成字节偏移），越界、切开字符、空的标注丢掉，每行最多 64 个，读音最长 32 字；和看懂自己的标注同类重叠的丢掉（`annotate.Merge`）。

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

给插件作者的规则（kando-plugins 仓库的 CONTRIBUTING.md 以这里为准）：

- 调 `kando.llm.chat`、`kando.sound.read`、`kando.net.credentialFetch` 时给 `host.call` 传第三个参数 `{ timeout: 150000 }`。`host.call` 不传时只等 30 s，而宿主替插件等 AI 服务、ffmpeg 或凭据请求最多 120 s，到时以 `timeout` 拒绝；不传的话插件会先收到运行时的 `ETIMEDOUT`，听不到宿主的回答（3.3）。`llm.chat` 上的超时不计入熔断。
- 用到的每个 handler 都在 `capabilities.host` 或 `optional` 里声明，没用到的不声明；`kando-plugin preview` 会在 PR 评论里列出对不上的（4.3）。
- `kandoManifest` 是普通函数，返回和清单一样的 `capabilities`、`optional`、`contributes`、`credentials`、`settings`、`events`（4.3）。
- 密钥用 `credentials` 声明，不写成设置（7.3）。

宿主推给插件的事件（`Host.Send`）只带标量，正文要靠有权限的 getter 去取：

```json
{"name":"film.opened","data":{"film":"f_3a9c…","spoken":"ja"}}
{"name":"job.completed","data":{"film":"f_3a9c…","stage":"complete"}}
{"name":"llm.changed","data":{}}
{"name":"line.changed","data":{"index":41}}
```

`line.changed`（E15 起）只推给正在显示的面板视图（数据是 `{"index": 正在播放的行号}`，没有行时是 -1），并且要插件声明了这个事件、此刻有 `kando.line.current` 的授权；为函数载入的插件收不到它，需要当前行时自己调 `kando.line.current`。其余事件也同时推给面板视图。影片 id 是按插件派生的 `HMAC(pluginSecret, filmKey)`，不同插件之间关联不起来，也不泄露路径。同一个插件看同一部影片只有一个 id：`kando.film.current`、`film.opened`、`job.completed` 手里是视频在磁盘上的路径，`kando.saved.read`、`kando.words.read` 和学习导出手里是收藏句子存的库内相对路径，派生前都按收藏句子的写法换成库内相对路径（`study.KeyIn`，以打开的资料库为准，收藏还没读进来时也一样）。`film.opened` 的 `spoken` 和 `kando.film.current` 一样，语音识别还没听出语言时给用户指定的语言（`films.Heard`）。

`kando.llm.chat` 的处理流程：先查预算 → 过 `paletteTranslator` 的门控（离线、隐私、未配置）→ 在 goroutine 里调 `ChatWith` → 经 `a.update` 回来 Resolve。现有的 `ChatTranslator.Chat`（`packages/media/translate.go:230`）只返回字符串，不读 usage，不设 `max_tokens`，请求头也只有 Content-Type 和 Authorization（`:609-611`）。所以 P2.0（已实现）先在 `packages/media` 加一个 API：`ChatWith(ctx, ChatRequest{System, User, MaxTokens, Header}) (ChatReply{Text, Usage}, error)`，`Chat` 改成调它，行为不变。服务商不返回 usage 时，按字符数估算 token。请求头带 `X-Title: Kando plugin <id>`，用户在服务商的账单里能分辨出来。

### 5.4 外部接口

- CLI 在 `cli/cli.go` 的命令表里加一行 `plugin`，子命令有 `list`、`validate <file-or-folder>`、`install <file>`，都带 `--json`。
  - `install` 先用和 `validate` 一样的检查验过文件（`publish.Check`：Stage、`plugins.Validate`，脚本插件再按官方根验签，所以开发者插件在命令行里和 `validate` 一样以「不是由看懂信任的密钥签名」拒收，开发者插件在 app 的开发者模式里装），再用 `plugins.Drop` 复制进 `plugins/inbox/`（先写隐藏文件再改名，文件名带随机前缀），用户在 app 里确认后才会安装（P1.1 已实现）。只接受文件，文件夹报用法错误，文件夹可以拖到 设置 › 插件 页上安装。app 在 `ready` 和 `becameActive` 时用 `plugins.Inbox` 取最早的一个文件，Stage 之后、弹出确认时把它移出 inbox（装不了的也只提示一次）；确认框开着或别的安装在跑时先不取，关掉后接着取下一个。看 inbox 用自己的 `inboxBusy`，不占 `busy`，所以切回前台时插件页的开关和按钮照常可用；看的过程中用户开始了别的安装或开关，inbox 让路，文件留到下一次；开关存完 state.json 后会马上再看一次（`pluginsSetEnabled`），安装和卸载之后的重新读目录也会看。
  - `validate` 离线运行，不连 app：把包 `plugins.Stage` 进一个临时目录（和安装走同一条解压、复制和 `plugins.Validate`），打印 id、kind、版本、英文名和内容哈希，然后删掉临时目录。（P0.3 已实现）
  - `list` 也不连 app：直接 `plugins.Scan` 插件目录，按 state.json 标出开关，并在 stderr 列出不是有效包的目录。（P0.3 已实现）
  - dev root 的生成和开发者包的签名不放在 CLI，也不经过 socket（见 6.4）。原因有两个：CLI 进程没有 NSGo Scope，拿不到 `core.Secrets`；socket 对同用户的任何程序都开放，不能让它替别人签包。
- 控制 socket 只加只读的 `plugins.list`，经过 `lockRemoteGate`。不加 install、enable、grant。（已实现：`remote.MethodPlugins`，回 `[]remote.Plugin`，每项是 id、kind、版本、英文名、内容哈希、是否启用，脚本插件再加来源，和设置 › 插件页的列表一致，不含目录、设置和密钥；CLI 和 MCP 没有接它，`kando plugin list` 仍不连 app、直接读插件目录。）
- 浏览器扩展、MCP 都不能触发安装或授权。`kando://plugin/install?id=<id>`（P5，第 15 节）只能为签名索引里列着的包打开安装确认：它让看懂按新索引下载那个包，之后走和「安装」同一条路径（校验大小和 SHA-256、Stage、Validate、脚本插件验签），没有用户在确认框里点「安装」就不安装，链接本身也不授予任何权限，授权只来自用户在确认框和详情页里的勾选；不在签名索引里的 id 不下载。

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
  - 数据包：glossary、dictionary、wordlist、ai-service、translate-style、export-template 安装后默认启用。`task` 要消耗 token，必须先授 `kando.llm.chat`（P2.2 的做法：task 包装好后默认关，用户打开开关就是授权，确认框和详情里写明每日预算）。`site` 必须逐个域名同意。
  - 脚本：安装后处于停用状态，详情页的能力一项都不勾，用户勾选并打开开关才算授权。能力之间没有依赖，每一项单独勾：每个 handler 只查它自己那一项（`llm.chat` 不读字幕，插件要把字幕交给 AI 就得另外申请 `lines.seen`，各自授权），所以勾一项不会连带勾上别的。
- 授权记录是完整的 `Grant`：权限名、network 规则的完整集合、每个凭据的 (id, hosts, header, format, methods, pathPrefixes)，以及站点域名。不再只存权限名。
- 实际生效的能力 = 包签名里的 `Capabilities` ∩ 清单声明 ∩ 用户授予的 `Grant` ∩ 当前状态。
  - 离线时 `Grant.Network` 为 nil，`llm.chat` 和 `credentialFetch` 拒绝。
  - 库被锁住时（与 `lockRemoteGate` 同义），所有读片库的 handler 拒绝。
- 授权写在 `<UserConfigDir>/Kando/plugins/state.json`，绑定内容哈希和签名者。它不进 settings.json，因为那个文件会整份重写、会丢掉不认识的键，而且离明文 key 太近；也不同步到手机或账号。
  - 包变了一个字节：如果新版本由同一个签名者签名，并且新声明的每一部分（权限、network 规则、每个凭据的 hosts/header/format/methods/paths、站点）都是旧授权的子集，授权沿用。任何一项超出旧授权，就视为扩大能力：插件按「旧授权 ∩ 新声明」继续运行，凭据只要有变化就整体暂停注入，并标出「已更新，需要重新授权」。
  - 签名者变了：授权、Storage 和凭据分区全部作废，按新安装处理。
- 每个插件的配额：每分钟最多 60 次 `host.call`（用户按下的导出期间读句子的声音另计，见 5.3）；`llm.chat` 默认每天 50k token，可以调整，用量在详情页可见；同时启用的脚本插件最多 12 个。
- **网络能力本身就是副作用通道**。有了 `lines.seen`（字幕内容可能来自攻击者）和 `llm.chat`，插件就能按 LLM 的输出发出有副作用的网络请求，不经过任何确认。所以 T6 防不住，只能缩小它的影响：
  - 安装页和详情页把「读你的字幕或用你的 AI」与「联网或 credentialFetch」同时勾选的组合单独标成高风险，单独一行说明。
  - network 规则必须写死 Methods 和 Paths/PathPrefixes（4.2 节，由 Validate 强制）。审核规范要求插件用最窄的规则；像 AnkiConnect 这种所有动作共用一个端点的服务，规则缩不到动作这一级，在审核规范和详情页如实写出来：规则的路径是 `/` 或路径前缀是 `/` 时（`plugins.Unscoped`），安装确认和详情页在联网那一项下面写一行「<host> 的所有操作都走同一个地址，这条规则限制不了插件做哪些操作」。
  - `credentialFetch` 只允许授权凭据里列出的 methods 和 pathPrefixes。
  - 真正只有宿主能拦住的副作用，才做成「每次确认」（`openURL`）或「只出建议」（`glossary.suggest`：复用 AI 服务术语建议的「添加」「不用」和撤销，见 5.3；`candidates.offer` 没有做）。

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
- P3b 的做法：凭据的密钥存在 `pluginhost.<id>.<signer>` 分区的 `credential.<凭据 id>` 下，详情页「密钥」一组为每个凭据画一个 secret 字段，下面一行「此密钥只会发送到：<hosts>」；授权时凭据跟着 `kando.net.credentialFetch` 这一项走，打开它就按当前声明授予全部凭据，Grant 里存完整的 (id, header, format, hosts, methods, pathPrefixes)。注入只在 https、默认端口、URL 不带用户名、host 是凭据的主机之一（不区分大小写）、method 是其中之一、路径按整段落在某个前缀下（`/v2/translate` 管 `/v2/translate` 和 `/v2/translate/x`，不管 `/v2/translated`）时发生；插件自己的头不能是凭据的头，也不能是 Host、Cookie 这类（Content-Type 可以）。host 表是 `plugins.Services`（DeepL、Notion、OpenAI、Anthropic、Gemini 等），凭据 id 或英文标题含其中某个名字而 hosts 不全在它的表里时，安装页和详情页标红；非官方插件的 hosts 不全在表里时也标一行提醒。
- 把 Kando 自己的 LLM key 从 settings.json 迁进 `core.Secrets` 是值得做的加固，但它不是插件系统的前提，列为独立 PR，由 owner 决定（见第 12 节）。

### 6.4 完整性与签名

- 脚本包只接受能链到受信根的 `.nsgopkg`（`pkg.Trust.Verify`，Trust 目录为 `<UserConfigDir>/Kando/plugins/trust`）。
- **根密钥每次启动都要重新加入**：`Trust.AddRoot`（`tsx/pkg/trust.go:66-72`）只在内存里追加，`OpenTrust` 只恢复吊销列表和已接受的 build。所以 app 每次启动都要先 AddRoot 内嵌的官方插件根公钥；开发者模式开着时，再加入存在 `core.Secrets` 里的 dev root 公钥。dev root 公钥不放普通文件，这样同用户的程序没法悄悄塞进一把自己的根。
- Kando 新建一把离线的插件根密钥，和应用更新用的 `NSGO_UPDATES_KEY`（`nsgo.yaml:31-33`）分开。用 `go tool nsgo certify` 给发布密钥签证书，限定 app-ID 前缀：官方是 `ai.yldm.kando.plugin.kando.*`，第三方是 `ai.yldm.kando.plugin.<publisher>.*`，和 id 的发布者段对齐（第 4 节）。证书有效期不超过一年。注意证书有效期是拿包头里签名者自己写的 `CreatedAt` 去比的（`tsx/pkg/package.go:428`），不是验证时的时间：过期前签的包过期后照样能装，发布密钥被偷时小偷也能把 `CreatedAt` 写进有效期内。所以有效期只限制正常的发布节奏，发布密钥泄露只能靠吊销（`revoke -keys`）。生成、签证书、签包和吊销的具体命令见第 15 节的 P3.0 测量。
- 吊销列表用 `go tool nsgo revoke` 生成，由 owner 在保管根密钥的机器上签好、提交进仓库（`apps/kando/plugins/revocations/kando-plugin-revocations.json`），每次发布由 Release 工作流检查后作为 `kando-plugin-revocations.json` 附在发布里，和 appcast 放在一起（P3c，命令见第 15 节）。社区目录的 CI 工具 `cmd/kando-plugin` 的 `validate` 和 `index` 可以用 `--revocations <文件或 https 地址>` 按同一份列表检查（P5 目录发布）：列表吊销了包或签它的发布密钥时，`validate --json` 给出 `revoked`，`index` 拒绝把它列进索引；地址回 404 时当作没有列表，和看懂一样；文件不存在时命令失败，免得路径写错就悄悄不检查；不给这个参数就不读、也不联网。`Trust.Accept` 持久化已接受的最高 build，用来防回滚。
- 联网时每天拉一次吊销列表（P3b 已实现，`state_plugins_revoke.go`）：地址是 `script.RevocationsURL`，即最新发布版里和 appcast 放在一起的 `kando-plugin-revocations.json`（`nsgo.yaml` 的 `updates` 旁有一行注释指向它）；还没发布时这个地址回 404，按「没有列表」处理，插件照常运行。拉取时间记在插件目录的 `.revocations-checked`，失败一小时后再试。列表过期时，只暂停申请了网络或凭据的非官方插件（依据 `Package.ExpiredRevocations`），不在全局调用 `RequireFreshRevocations`，免得长期离线的用户连纯本地插件都用不了。被吊销的插件立刻 Close，并显示「已被撤回」。
- 可选的加固：把每个根已见过的最高吊销序号，以及每个插件已接受的最高 build，额外存一份摘要到 `core.Secrets`。启动时和 `plugins/trust/trust.json` 比对，发现回退就停用所有非官方脚本插件。是否做由 owner 决定（见 12.3 第 2 条）。
- 来源徽标（官方、已验证、开发者、本地数据包）由证书链推出，不读清单。插件名去掉 Bidi 控制符和零宽字符，最长 40 个字符。保留名单是「Kando」「看懂」「Official」「官方」「公式」「공식」，再加上所有内置 AI 服务商的名称（取自 `settings.Presets` 的显示名），比较时忽略大小写、全半角和空白。`plugins` 包不能导入界面包，所以名单以静态列表 `plugins.ReservedNames` 存在包里（含五种界面语言下的服务商名称），根目录的 `plugins_test.go` 断言 `settings.Presets` 在每种界面语言下的每个名称和 `appName` 都在名单里，内置服务商改名或新增时测试会失败。
- **开发者模式**是设置里的显式开关。dev root 的生成和签名都在 app 里完成，位置是设置 › 插件 › 开发者：「生成开发者密钥」把私钥和公钥都存进 `core.Secrets`；「打包并安装文件夹…」由用户在系统文件面板里选一个源码文件夹，app 用 dev key 签名，然后走和正式包同一条 Stage → Verify → 比对 kandoManifest → 确认 sheet 的路径。不允许用 `tsx.Sandboxed` 直接跑源码，CLI 和 socket 都不能签。开发者包一直带着徽标，不参与更新，不能分享安装，也不允许占用已安装的非开发者包的 id。
- 数据包可以不签名。将来如果支持签名数据包，site 包的高风险能力只对官方和已验证来源开放。

### 6.5 安装与更新

- 安装入口有四个：设置 › 插件页的「从文件安装…」、把文件或文件夹拖到 设置 › 插件 页上（`desktop.DroppedPaths`）、CLI inbox，以及 P5 的「插件目录」（目录里的「安装」「更新」，和 `kando://plugin/install?id=<id>` 链接，链接只是替用户在目录里点了这一行，见 5.4 和第 15 节）。目录按签名索引下载、校验大小和 SHA-256 后，走和选中文件同一条路径（`pluginsIndexFollow`：Stage、脚本插件验签、`pluginsAskStaged`）。四者最后都要经过宿主画的确认框。开发者包另有 6.4 节的入口，同样要经过确认框。拖进主窗口别处的文件由片库那块的 `dropFiles` 接收，要在那里分出插件包就得往别的块的函数里插调用（AGENTS.md 不允许），所以主窗口的拖放安装没有做，只有插件页本身接拖放。一次拖进多个文件时只问第一个。
- 安装流程：
  1. `plugins.Stage`：复制或解压到 `plugins/.staging-<rand>`。zip 按实际字节计数（嵌套 zip 共用计数器），拦截 zip-slip、符号链接和绝对路径；文件夹用 Lstat 逐项复制。
  2. 对 staging 里的副本跑 `plugins.Validate`，包括 id 正则和路径检查。脚本包还要 Verify，并在预读沙箱里比对 `kandoManifest()`。
  3. 静态检查：声明的 host 权限都在已知 handler 表里；签名 Capabilities 里除了 Host 和 Network 没有别的字段。
  4. 用户在确认 sheet 上点「安装」后，在引擎 goroutine 上关掉同 id 包的句柄，再 `plugins.Swap`（5.1 节的顺序）。
  5. 残留的 `.staging-`、`.old-` 目录超过 24 小时才清理。清理由每次 `Stage` 开始时顺带做，不另设入口。rename 会保留目录原来的修改时间，很久以前装的包刚被挪开时看起来也已经过了 24 小时，所以挪开的目录命名为 `.old-<挪开时的秒数>-<随机串>`，`.old-` 目录的年龄从名字里的时间算起，免得另一个 Kando 的 `Stage` 删掉 `Swap` 回滚还要用的旧包。
- 每次启动 Scan 时，用同一个校验器再校验一遍。清单、字段和目录形状每次都查；内容哈希（和词典嵌套 zip 的检查）按包里每个文件的（文件名、大小、修改时间）缓存在插件目录的 `.hashes.json`（`plugins/hashcache.go`，写临时文件再 rename），都没变就沿用上次的哈希，不再把可能解开有 1 GiB 的词典读一遍，`kando plugin list` 也一样。修改时间离这次 Scan 不到 3 秒的文件不缓存（文件系统的时间粒度可能只到秒），下次再读；缓存的格式号变了、读不了或超过 4 MiB 就全部重算，校验规则一旦改到文件内容（哈希、上限、嵌套 zip 的检查）就要升 `hashCacheFormat`。同用户的程序可以改了文件再把大小和时间改回去，让旧哈希继续有效；它本来就能改 state.json，所以这不扩大 6.7 的风险。`Stage` 和 `kando plugin validate` 仍完整校验。
- 第一版不做在线更新：更新就是安装一个新文件，走同样的确认流程，授权按 6.2 节的子集规则处理。P5 的社区索引（第 15 节）也不自动更新：签名索引里列出的版本比已装的新时，「浏览插件」显示「更新」，点了照样下载、走同样的确认流程和子集规则；离线模式下不检查。
- 插件文件永远不写进 `.app` 包，只放在 `<UserConfigDir>/Kando/plugins/`。

### 6.6 隔离、熔断与审计

- 每次调用都包在 recover 和超时里，失败就整段丢弃这次的产出。
- 熔断按 (id, hash) 计：init 失败立即停用；最近 50 次调用里有 3 次出错或超出 `ErrBudget`，也停用；`llm.chat` 上的 `ErrHostTimeout` 不计入。tsx 自己停掉的运行时（超了节点或内存配额）之后的每次调用都立即以那个配额的带码错误失败，熔断不计带码的错误，所以调用以这种错误结束时马上停用插件：内存配额记 `memory`，其他记 `failed`。Kando 目前不设 `Limits.Memory`（`TODO(nsgo#760)`），所以现在只有节点配额会走到这里，记 `failed`；`memory` 留给以后设了内存配额的时候。认法是错误带着配额的资源名（`Resource`）：tsx 把插件 throw 或 reject 的错误只留码和消息，插件自己报同样的码不会被当成停了（`runner.go` 的 `halted`）。只看错误、不调插件的任何东西：插件可以在自己的导出对象上挂 getter，tsx 在引擎上读名字时会不限时地跑它。结果写进 `plugins/health.json`，只存类别。
- **卡死和崩溃后自愈**：每次 Call 之前先把 (id, hash) 写成 in-flight 记录，回调时清掉（5.2 节）。下次启动时如果发现残留的记录，就把那个插件标为 `hung` 并自动停用，详情页说明「上次运行时让看懂停止响应」。这一条覆盖了 Limits 拦不住的卡死和 OOM（见 6.7）。
- **看门狗**（P3.0 加的，Limits 管不到的部分）：所有脚本插件的 Call 全局串行，同一时刻只有一个在跑，引擎变慢和内存上涨都能归到它头上。一个 goroutine 定时往引擎投心跳，并用 `runtime/metrics` 读 Go 堆；有调用在跑时，心跳连续几次等待超过 250ms，或者堆比 Load 前的基线多出 512 MiB，看门狗就在自己的 goroutine 上 `Plugin.Interrupt` 这个插件（nsgo#836 加的，任何 goroutine 都能调），正在跑的片段当场被切断、定时器丢掉，再在引擎上 `Plugin.Close()`，记一次熔断并停用。以前只能 Close，要等到两个片段之间才生效（实测 0.05–0.4 s）；现在不用等当前片段跑完（goja 执行时检查中断标志；原生的正则匹配不检查，由匹配超时兜住）。tsx 的 `Limits.CallTotal` 不用：它是从 `Plugin.Call` 起算的墙钟时间，宿主替插件等（`Pause`、`PauseFor`，读声音、`llm.chat` 最长 120 s）时停不下来，而看门狗的 1 分钟期限不算这段；它还会让入口模块排下的定时器在 `Limits.Load`（1 s）后被丢掉，插件作者可能依赖这一点。正则由 tsx 兜住：沙箱里的调用编译的正则以这次调用的预算（500ms）为匹配超时（nsgo#759 修好后，Kando 不再设进程级的 `regexp2.DefaultMatchTimeout`，6.7）。
- `plugins/audit.log` 记录安装、授权、吊销、熔断、确认框结果，以及每次 `llm.chat` 的 token 数，不记录内容。写盘前先经过 `feedback.Scrubber`。数据包也记：每种包的安装（`install`，带版本和 kind，site 包带安装时同意的域名）和卸载（`uninstall`）；安装确认没装就关掉（`cancel`，打不开的文件不算，它没问过用户）；site 包在详情里逐个域名的同意和撤回（`site`）；task 包的开关（`enable`、`disable`，打开它就是授予 token）和每次运行花掉的 token（`llm-chat`）。其余数据包的开关不是授权，不记。

### 6.7 残余风险（在授权页如实写出）

- **恶意或写坏的插件可以让窗口永久卡死，或者让整个 app 内存耗尽**，Limits 兜不住这两种情况。第一，时间预算靠 `vm.Interrupt` 实现，但 goja 的回溯正则走 regexp2（`goja/regexp.go:58`），没有设 MatchTimeout（`regexp2` 的 `DefaultMatchTimeout` 是 `math.MaxInt64`），执行原生匹配时也不检查中断标志。一句 `/(a+)+\1$/.test("a".repeat(40)+"!")` 就能让引擎 goroutine（也就是 UI 线程）一直卡住，500ms 的预算不会触发。这一点已用 goja 最小复现确认：200ms 时发出 Interrupt，10s 后匹配仍在运行，已提交为 [nsgo-dev/nsgo#759](https://github.com/nsgo-dev/nsgo/issues/759)，上游已修：tsx 在每个沙箱调用期间把 `regexp2.DefaultMatchTimeout` 设成这个运行时 `Limits` 的 Call 与 Load 中较短的那个（Kando 是 Call 的 500ms；`tsx/sandbox.go:255-259` 定这个值，`:66-93` 的 `limitRegexps` 在几个调用嵌套时取最短的，没有调用时恢复原值，`:481-483` 只在最外层的 `within` 设），匹配超时的 `RangeError` 按 `ErrBudget` 报（`sandbox.go:286-295`），所以 goja 仍不响应 Interrupt，但调用里编译的正则最多匹配一个预算。第二，`Limits` 没有任何堆内存上限，一个同步片段在 500ms 内就能分配出几个 GB，已提交为 [nsgo-dev/nsgo#760](https://github.com/nsgo-dev/nsgo/issues/760)。P3.0 用 Kando 的 Limits 实测：同一句正则 n=24 卡 4.2 s、n=28 卡 71 s、n=32 超过 300 s（推算约 19 分钟），预算要等匹配结束才报 ErrBudget；一个同步片段在 500ms 内能分配 0.15–0.46 GB；用 setTimeout 分片的调用永远不被切断，5 s 涨到 2 GB（3.3 节）。Kando 自己能做的缓解有两条，都已实测有效：一是在 main 里、任何 Load 之前设 `regexp2.DefaultMatchTimeout = 100 * time.Millisecond`（goja 用 `regexp2.Compile` 编译每个 JS 正则，`goja/regexp.go:58`，超时后抛 `RangeError`，`:138-139`），n=32 的匹配 0.2 s 就返回，nsgo#759 修好后不再设，改由 tsx 按调用的预算兜住，n=32 的匹配不到 1 s 就以 `ErrBudget` 失败（`TestScriptRegexpIsBounded`）；二是 6.6 节的看门狗。剩下的残余风险是：最坏卡一个片段（500ms）加一次匹配超时，最多多占约 0.5 GB；看门狗漏掉的情况（例如它自己的阈值调得不对），仍靠 6.6 节的 in-flight 自愈（下次启动时自动停用）和签名问责。P3 的绕行处写 `// TODO(nsgo#759): ...` 和 `// TODO(nsgo#760): ...`，带上 issue 链接；nsgo#759 修好后它的 TODO 已随绕行一起去掉，只剩 `runner.go` 的 `TODO(nsgo#760)`。
- **本机状态文件对同用户可写**：同用户的进程可以改写 `state.json`（授权）、`health.json`（熔断），以及 `plugins/trust/trust.json`（已接受的最高 build 和吊销列表）。删掉或改掉 trust.json，防回滚和吊销就会清零：`OpenTrust` 在文件不存在时返回一个空的 Trust，而 "a root without a list revokes nothing"。所以「防回滚」和「已被撤回」只防得住网络上的攻击者，防不住本机程序。msime 尝试过 keychain HMAC，最后放弃了，这里不假装能防住。6.4 节的可选加固能把回退检测出来。
- **凭据库按平台不同**：macOS 钥匙串条目的 ACL 信任写入它的那个二进制，其他同用户进程读不到插件的凭据。本设计让分区带上签名者、dev root 公钥也存进钥匙串，就是为了不让同用户程序借「同 id 换包」或「塞一把自己的根」拿到这些凭据。Windows 凭据管理器和 Linux Secret Service 对同用户进程本来就开放，在这两个平台上，同用户的恶意程序能直接读出插件凭据，插件系统不额外放大也无法缩小这个风险。
- **网络是副作用通道**：获准联网的插件，可能被字幕内容诱导着，发出它声明范围内的任何请求（6.2 节）。
- goja 本身也可能有其他漏洞，不是零风险。
- 授权页写出的是用户在授权时能据此决定的几条：第一条在每个脚本插件的安装确认和详情里各一行（「插件卡住或占用太多内存时，看懂会停用它，但在那之前窗口可能停顿片刻」）；凭据库一条在 Windows 和 Linux 上、插件声明了凭据时，写在安装确认的凭据下面和详情的「密钥」一组里；网络一条就是 6.2 的高风险一行。本机状态文件和 goja 自身漏洞这两条与具体插件无关，授权时也没有可选的做法，只写在这里，不上授权页。

## 7. UX

### 7.1 入口与列表

- 设置里新增一节「插件」：`settings/sectionid` 加 `Plugins`；`settings/sections.go` 的 `var sections` 加一行（不需要改成函数，插件页一直显示）；页面写在 `settings/plugins.go`，用 `settings/form` 的 Card、Label、Badge、Button 和 `ui.Switch`、`ui.IconButton` 拼。它读自己的接口 `settings.PluginsApp`，不往 `settings.App` 里加方法，照 `Window.Account` 的先例由 `settings.Window.Plugins` 注入（`state_settings.go` 的 `settingsWindow` 里多一个字段）。
- 列表按 kind 分组。每行有名称、来源徽标、版本·作者、行内开关，以及展开详情的按钮（P1.1 没有每行的图标）。P1.1 的状态只有开关本身和「未安装」：savedPlugins 引用了、目录里却没有的包单独列在「不在插件文件夹里」一组（包已经读不到，不知道它的 kind），带「移除」按钮。其余状态到有对应机制的那一期再加：需要授权 / 已更新，需要重新授权 / 已停用：出错 / 已停用：太慢 / 已停用：上次让看懂停止响应 / 已被撤回 / 离线时不可用。脚本插件的这些状态都代替版本号写在行内，详情顶上再写一遍：验签没过、已停用的原因、已被撤回、吊销列表过期，然后是「需要授权」（有一项不是可选、需要授予的权限没授予，或者网络规则没授予，`plugins.Missing`，刚装好什么都没授予时也是它）和「离线时不可用」（离线模式开着，插件必需的有网络规则、`credentialFetch` 或 `llm.chat`，`plugins.Online`），同一时刻只写排在最前的一条。「已更新，需要重新授权」写在详情里。
- 词典分组里每行有上移、下移两个按钮（`lucide.ArrowUp`/`ArrowDown`，在第一行、最后一行时禁用），决定查词时谁先答，顺序存在 `savedPlugins.PluginOrder`。NSGo 固定版本没有可拖动排序的列表控件，所以不做拖动。移动只改顺序，行按包 id 保留并移动（`core.Each`），不重建，按下的按钮保住键盘焦点，可以连按几次。
- 顶部按钮：「从文件安装…」（系统文件面板，配合 `holdModal()`，接受 `.zip` 和 `.kandopack`），以及「在访达中显示」（按平台换成文件资源管理器、文件管理器；`desktop.Reveal`，经 `revealInFolder`，插件目录不存在时先建出来）。开发者模式开着时，多一组「开发者」：生成开发者密钥、打包并安装文件夹…。
- 详情在行下面展开：说明、它会加什么（术语条数和语言、AI 服务的 host 等，由 app 用界面语言写好）、许可、主页、词典的 attribution、id 和哈希前 12 位，以及「卸载…」。卸载的确认也在详情里展开（和手机同步「撤销…」的做法一样），确认后删除目录并从 state.json 移除。state.json 是更新版本写的时，和安装一样在删之前就拒绝，插件页另写一句「没有卸载」（`plugins.ErrUninstallRefused`，和列表上方说明 state.json 的那句不重复）；目录已经删掉、之后 state.json 没存上的，算卸载好了，插件页另写一句「已完成，但没能保存哪些插件是启用的」（`plugins.StateUnsaved`，安装后没存上也是这一句；原因是插件页认得的错误时用它自己的文案），不说卸载失败。
- state.json 读不了或是更新版本写的，在列表上方用一行红字说明所有包暂时停用。
- 问题区列出没能载入的包，以及原因的本地化文案。

### 7.2 安装确认（`settings.PluginInstallDialog`，`ui.Dialog`）

P1.1 的确认框只有数据包要显示的部分：名称（标题）、说明、它会加什么（AI 服务包写明「会把你的 API Key 和要翻译的字幕发送到 <host>」）、来源（「来自这台电脑上的文件 · 未签名」）、版本·作者、许可、id 和哈希前 12 位，以及要替换的已安装版本。取消是默认按钮，回车和 Esc 都不安装。点了「安装」之后、安装结束之前，取消按钮变灰，回车和 Esc 也不起作用：这时 `plugins.Swap` 正把暂存副本移到位，取消会把它删掉，留下一个用户取消了却装上的包，或者一个缺文件的包顶替了旧版本。文件装不了时，同一个对话框只说原因（catalog 文案）和「好」；点了「安装」之后才失败的（另一种 kind 已有这个 id、写盘失败），确认框也不关，换成同样的失败视图；state.json 是更新版本写的时，在 `plugins.Swap` 之前就拒绝，什么都不替换。包已经放到位、之后 state.json 没存上的，算装好了：确认框照常关掉，存不上的原因单独显示在插件页上（`plugins.err`，「已完成，但没能保存哪些插件是启用的：<原因>」）。失败视图等用户点「好」，因为它可能画在主窗口上、插件页根本不在眼前。确认框开着时占用 `holdModal`。它的文字在建的时候取，所以界面语言切换时整个重建（读 `i18n.Language(cx)`，和 `settings.Window` 一样）。它画在哪里：设置在自己的窗口里、并且正停在插件页时，由插件页画；其余情况（CLI inbox、设置画在主窗口上、设置开在别的页）由主窗口的 `pluginsOverlay` 画（`PluginInstallHere` 决定），免得确认框被设置窗口挡在主窗口里。下面几条是脚本插件（P3）要加的部分。


- 显示名称、来源（例如「由 acme 发布，看懂已验证」，取自证书）、版本、大小、哈希前 12 位。大小是暂存副本里所有文件的字节数（`plugins.Size`，在后台和验签一起算），写在版本和作者后面（「版本 1.2.0 · Acme · 12 KB」），只有脚本插件写。
- 「它想要」的清单：每个权限的说明来自 catalog，插件不能自定义权限说明。例如「读取正在播放的影片到当前句为止的字幕」「用你的 AI 服务回答，每天最多 5 万 token」「向 api.deepl.com 发送 POST /v2/translate」。必需权限在上，可选权限在下，默认不勾。
- 「读你的字幕或用你的 AI」与「联网」同时出现时，单独一行标为高风险。凭据一行写明它会发往哪些主机；主机不在内置服务表里时额外警示。网络规则缩不到动作时写一行（6.2），高风险一行之后写 6.7 的残余风险，凭据下面在 Windows 和 Linux 上写其他程序也能读到密钥（6.7）。
- 确认框里的权限仍按必需、可选两组列出，不按 7.3 的四组：安装时还没有开关，用户要看的是装了之后它缺哪些就不能用。
- 开发者来源加一条警示。site 包每个域名一行复选框，默认不勾；重装同 id 的新版本时，旧版本已经同意、新版本仍然列出的域名预先勾上（P4.1）。
- 点「安装」后，脚本插件仍处于停用状态，回到列表打开开关才算授权。

### 7.3 详情页

- 说明和出处（词典必须显示 attribution）。
- 权限按「读取你的内容 / 改动你的内容 / 联网 / 用你的 AI」分组，每一项都能单独撤回。分组是 `plugins.Perm.Group`：读影片、字幕、收藏、生词、声音和它自己的设置是「读取」；`kando.toast` 在窗口里显示提示，是这一版插件唯一能改的东西，归「改动」；网络规则、`credentialFetch` 和 `kando.ui.openURL`（打开的网页会收到网址里的内容）归「联网」；`llm.chat` 归「用你的 AI」。组里必需的在前、可选的在后，没有权限的组不画标题。联网一组下面写 6.2 的「规则限制不了」一行，权限之后是高风险一行、按键冲突和 6.7 的残余风险一行。
- 每日 token 用量和预算。
- 插件声明的设置，类型有 bool、choice、number、text，最多 16 项，由宿主用 `settings/form` 加 `ui.Switch`、`ui.Select`、`ui.Input` 渲染。值存在 `plugins/.settings/<id>.json`，不计入哈希，改设置不需要重新授权。密钥不是设置：`Validate` 拒收 `secret` 类型的设置，原因里让作者改用 `credentials` 并申请 `kando.net.credentialFetch`。凭据在「密钥」一组里各画一个 secret 字段，输入框下写着「此密钥只会发送到：<hosts>」，密钥存在 `pluginhost.` 分区里（6.3）。
- 按键冲突提示、「查看审计记录」、「卸载」。卸载会删除目录、`plugin.<id>.*` 和 `pluginhost.<id>.*` 两个分区，并从 state.json 移除，删除前要确认。AI 预设插件留下的服务记录会保留，key 不丢。

### 7.4 角色选择的位置

- 设置 › 字幕外观 最后的「逐词标注」卡片（`settings.Marks`，读自己的 `settings.MarksApp`，照 `Window.Plugins` 的先例由 `settings.Window.Marks` 注入，不扩大 `settings.App`）：「注音或拼音」开关（说明写明日语假名要启用日语词典包、或在「按等级画线」里选日语词表，只装了日语词表包而没选它不会读大词典）、「按等级画线」下拉框（「关闭」加上已启用的词表包，名称取包的 `name`；说明写明会在词下面画一条该等级颜色的粗线）、「标出已学词」开关（说明写明是虚线）。三项默认都关。（看懂的 设置 › 通用 › 外观 只管明暗，所以放在字幕外观。）
- 查词卡在底部标出来源，例如「JMdict」或「AI」。学日语或中文的用户第一次点词时，卡片底部提示「装离线词典，查词更快、不耗 AI」，点了打开设置 › 插件。条件是这个词是日语或中文（`lookup.WordLanguage`），插件目录已经读过，并且没有装这种语言的词典包（开没开都算装了）；只提示一次，提示过就记在 settings.json（`savedPlugins.PluginDictHintShown`，`pluginsDictHint`）。
- 设置 › AI 的服务商下拉框里，插件预设排在内置预设之后，每项写成「插件 · <包名> · <host>」（`ui.Select` 还没有带标题的分组，见 E4）。点「获取 API Key」时先用系统提示框显示目标域名再打开。
- 导出字幕的格式下拉框、学习导出的目标列表里出现插件项，后面标出插件名。
- 插件经 `kando.glossary.suggest` 提议的术语列在播放器「术语」标签的「片中找到的术语」里，排在 AI 服务的建议后面，每条下面一行写「来自插件「<插件名>」」（`film.Glossary.TermSuggestionPlugin`），「添加」「不用」和 AI 服务的建议一样。
- 插件命令出现在命令面板和菜单栏。在 macOS 上，不认识的分组会由现有逻辑放在「窗口」菜单前面，单独成一个菜单（`menu.go:20`）；在 Linux 和 Windows 上，命令面板是唯一入口。

### 7.5 错误

- 插件失败不弹窗打断，在出错的地方说清楚，比如查词卡上写「JMdict 没有这个词，问了 AI」：先问过的词典（`lookup.Ask.Dicts`）都没有这个词、由 AI 服务回答时（查词出错的词典，比如索引坏了、经 `Failed` 报告的，不算「没有这个词」，不列进去），来源一行写「词典「<名称>」没有这个词，问了 AI 服务」，问过几本时写「你的词典都没有这个词，问了 AI 服务」（`vocabSourceText`）。缓存里的 AI 回答没有问过词典，照旧写「由 AI 服务解释」。手机的查词卡不写这一句（`psync.WordSource` 没有这一项）。
- 错误码在 `plugins` 包里是英文哨兵错误（`ErrUnknownKind`、`ErrUnsupportedKind`、`ErrBadSignature`、`ErrRevoked`、`ErrTampered`、`ErrUnknownKey`、`ErrTooLarge`、`ErrBadID`、`ErrNoSecrets`、`ErrHung`……），由 `bind_plugins.go` 映射成 catalog 文案，不把 `err.Error()` 拼进本地化句子。

### 7.6 多语言

- 看懂自己的文案按 AGENTS.md 写进 `apps/kando/locales` 的五份 catalog，前缀用 `settingsPlugins…`、`plugins…`，然后跑 `go tool nsgo i18n`，把 `catalog_gen.go` 一起提交。
- 插件提供的文字来自清单的 `Text` map，只在 `LabelFunc`、`ContentFunc`、`core.Dynamic` 这类绑定里用 `Text.In(i18n.Language(cx))` 解析（NSGo 的语言是字符串，见 `i18n/i18n.go:104`），这样切换语言时会跟着变，满足 `locales/follow_test.go`。找不到当前语言就沿 `i18n.Parent` 回退，最后用 en。
- 复数只支持 `i18n.Cardinal` 的 one/other，第一版不做。
- 插件文字一律当纯文本显示，并限制长度。
- 手机端：不显示插件，只同步插件的产出（glossary 已经体现在字幕里；标注和读音由桌面算好，经协议 v2 新增的 `films.marks` 发给手机，见 E2）。查词结果经 `vocabResolve` 由桌面算好，回给手机。不加新的 `Platform` 值，不改协议版本（`films.marks` 是同一版本下新增的调用）。

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
- `state_browse.go` 的 `browseNotVideo`、`browseList`、`browseEnqueue`、`browseHeard`，和 `state_browse_buttons.go` 的 `browsePageAsk`、`browsePageAdd` 接 `pluginSites`，`browseSitesChanged` 和 `browseTakeScript` 换页面脚本，web view 用新脚本重新载入当前页面，浏览器页在后台时等它再显示（见 E8）；`queue.go` 的 `queueRun`、`queuePipeline` 和 `queue_remedy.go` 的 `FailureRemedy` 对 `core.Job.PluginSite` 的任务不借用登录（P4）。
- `locales/*.yaml` 五份，加上 `catalog_gen.go`。
- `film/panel.go` 的 `panelTabs`（E15，已实现：`screen.panelTabs`，「扩展」一项按 `film.Extender` 出现）。
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
  - 九种 kind 现在都已实现（`Kind.implemented`），没有被拒收的。`ErrUnsupportedKind` 的路径留给以后新加的 kind：解析器认得、这个版本还不装的 kind 被 `Validate` 拒收，不报未知键；眼下没有这样的 kind，所以没有对应的测试。
  - Grant 子集比较（P3）：只扩 credential 的 hosts、只改 header 或 format、只加一条 network 规则，都应判为扩大能力；换签名者时授权作废。
- **对解析器做模糊测试**：`go test -fuzz` 覆盖 `Parse` 和 zip 解包，跑在 CI 里，带 `-fuzztime` 上限。
- **`dict` 和 `wordlist`**：用小型样例词典验证索引构建、二分查找、原形查询、CC-CEDICT 的拼音，以及 Close 之后 Swap 能成功；词表的注释、未知 tier、大小写、重复词和空词表。真实词典的测量（`dict.TestRealDictionary`）只在设了 `KANDO_DICT_BENCH`（词典文件的路径）时跑，CI 不下载词典，结果见第 15 节。
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
  - 用 `go/ast` 扫描整个仓库，断言 `tsx.Sandboxed` 一处都不出现；`tsx.Load`、`tsx.View` 只出现在 `plugins/script` 的非测试代码里，并且都经过 `script.Sandbox`（`tsx.View` 只能写成 `Sandbox` 字段是 `Sandbox(...)` 调用的字面量）。
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
- PR 2.5：应用内「把我的术语表或词表导出成包」，同一种格式，走同一个校验器。（已实现，见第 15 节）

### P3 脚本插件（只在桌面端，先过 spike 门槛）

- PR 3.0（spike，不合并，或者只合并测量脚本）：链接 nsgo/tsx，量出三平台的体积增量、引擎阻塞时间、in-flight 写盘开销，以及 ReDoS 和大分配的实际表现，结果报给 owner，由 owner 决定是否继续（2026-11-19 已测完，没有合并任何代码，结果和放行建议见第 15 节）。不达标就停在 P2，数据包的价值不受影响。
- 实际按 P3a、P3b、P3c 三个 PR 落地（第 15 节）：P3a 合并了下面的 3.1、3.2（除 `llm.chat`、`sound.read`）、3.3 和 3.6 里的开发者模式与 `validate`；P3b 是 3.4、3.5 和 `llm.chat`；P3c 是 3.7 和吊销列表的发布、拉取。
- PR 3.1：`plugins/script`：Trust（每次启动 AddRoot）、Verify、预读沙箱比对 kandoManifest、`Limits()`、Sandbox 构造、熔断和 in-flight 自愈（health.json），audit.log；`State` 的每一项加上 `Grant`、`Signer`、`OldHash`、`Budget`，`State.Format` 升一级；加上 Grant、Limits、Sandboxed 的不变量测试和假插件夹具。
- PR 3.2：每个插件自己的 host 和 handler（film、line、lines.seen、saved、words、sound、toast、settings.get），`llm.chat` 代理和预算（用 `ChatWith`），事件推送，逐个 update 载入。
- PR 3.3：命令贡献（`keys.go:83`），加运行时按键去重。
- PR 3.4：查词来源、导出格式（`Export.Cues`）、学习导出目标、标注器。
- PR 3.5：`credentialFetch`（methods 和 paths 限制、`pluginhost.` 分区、host 表警示），加上凭据的 secret 字段。
- PR 3.6：签名运维：插件根密钥、`go tool nsgo certify/revoke` 的发布流程、吊销列表随 appcast 发布、app 内的开发者模式（生成密钥、打包并安装文件夹）、CLI 的 `kando plugin install`（`list` 和 `validate` 在 P0.3 已经有了）。
- PR 3.7：官方参考插件 `kando.ankiconnect`，同时作为示例和 CI 夹具。

### P4 站点包

- PR 4.0：`core.ValidateVideoSite`。
- PR 4.1：site kind 和单独的 `pluginSites` 匹配器，只接内置浏览器（工具栏的「下载视频」和网页按钮的两个入口，见 E8），逐域名同意，不带浏览器 cookie，不支持 sign-in。加测试锁住 socket、浏览器扩展、remote 不认插件站点。设置页说明浏览器扩展不覆盖插件新增的站点。

### P5 按需单独立项

各项单独评审，不承诺：

- 候选译文和机器翻译来源（`media.StageError` 加 `Tools.Engine`）；
- 播放器侧栏的「扩展」标签和 `tsx.View`（已实现，第 15 节 E15）；
- 社区索引（原定数据包限速后即时上架、代码包审核后上架；建成的 `yldm-tech/kando-plugins` 里数据包和脚本插件都要维护者审核、合并后才上架，限速没有做，见 12.3 第 5 条；安装仍然不授予任何能力；客户端、打包工具和发布都已做，放在官网的域名下而不是原定的 R2，见第 15 节）；
- TTS。

## 11. 新依赖

P0–P2 不新增任何 module。P3a 起 `github.com/dlclark/regexp2/v2 v2.11.0` 成为 `apps/kando/go.mod` 的直接依赖（只用来设 `regexp2.DefaultMatchTimeout`，见 6.7；nsgo#759 修好后不再设，又回到 indirect），`go mod tidy` 多出 `github.com/google/pprof v0.0.0-20230207041349-798e818bf904`（indirect），两者的版本都是 NSGo 已经选定的，owner 已同意（12.3 第 3 条）。其余用到的都已经在 `apps/kando/go.mod` 里：

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

1. **tsx 和 goja 在 Windows 上只测了功能，没量性能**：P3.0 已量过二进制体积（三平台 +19–21 MiB，约 +37%）、Linux 和 macOS 上的引擎阻塞和内存（第 15 节）。P3c 起 CI 的 Test（Windows）之后有一步「Plugin runtime」，每次不走测试缓存地在 Windows 上跑 `TestPlugin|TestScript`（`./apps/kando/` 和 `./apps/kando/plugins/...`：运行时、预算、看门狗、in-flight 标记、官方插件），并且通过；Windows 上引擎单次阻塞的时间和内存仍没有量过。缓解：数据包两期不依赖它。
2. **插件跑在引擎 goroutine 上，而且卡死不一定能打断**：await 不计入预算（`sandbox.go:311` 的 `within`），同步片段有 500ms 预算；但 Limits 没有内存上限，ErrBudget 不停掉插件，用 setTimeout 分片的调用永远不被切断（3.3、6.7 节）。回溯正则原来不响应中断，[nsgo-dev/nsgo#759](https://github.com/nsgo-dev/nsgo/issues/759) 已在上游修好，固定的 NSGo 版本里 tsx 把调用的预算（`Limits.Call`）设成正则的匹配超时，看懂不再设进程级的 `regexp2.DefaultMatchTimeout`（`TestScriptRegexpIsBounded`）。缓解：6.6 节的看门狗（全局串行、心跳和堆阈值、在片段之间 Close）、in-flight 记录加下次启动自动停用，签名问责，重计算（分词、注音）留在 Go 原生实现，Load 逐个 tick 进行；上游 [nsgo-dev/nsgo#760](https://github.com/nsgo-dev/nsgo/issues/760)（没有内存上限）仍开着；[nsgo-dev/nsgo#836](https://github.com/nsgo-dev/nsgo/issues/836)（单次调用没有总时长、宿主不能打断插件）已在上游关闭，看懂还没改用，看门狗照旧（`plugins/script/runner.go`、`panel.go` 的 `TODO(nsgo#836)`）。让插件跑在自己的 goroutine 上（独立的 `core.NewRoot`）P3.0 试过可行，但插件的 Storage 只能留在内存里（第 15 节），所以不作为默认做法。
3. **签名元数据缺口**：[nsgo-dev/nsgo#761](https://github.com/nsgo-dev/nsgo/issues/761) 已在 2026-11-19 关闭、在 `f377daba0a2a` 修好，看懂认包里签的清单，不跑代码就能比对（4.3）；但 `nsgo publish` 还签不了它，已发布和新发布的包仍靠预读沙箱里比对 `kandoManifest()`，插件的顶层代码仍在用户同意前运行。
4. **`tsx.Sandboxed` 和共享 Host 才是绕开权限的入口**：`tsx.Sandboxed` 在 Capabilities 和 Grant 都为 nil 时拿到宿主的全部权限，连 `file://` 都能读；Load 路径上的 nil Grant 会让包声明的能力绕过用户勾选生效；handler 只要用 `a.cx` 调 NSGo 服务，就带着 app 的权限。缓解：AST 测试全仓禁止 `tsx.Sandboxed`；Grant 由唯一构造函数从零构造，并测试零值字段；每个插件独立的 Host；每个 handler 都查 `Effective`。
5. **site 包**：已经改成独立的 `pluginSites`，不放宽全局闸门；剩下的风险是 JS 正则的校验漏了嵌套量词时会让页面卡顿，以及用户对「浏览器扩展不覆盖插件站点」的预期要讲清楚。插件正则现在也写进内置浏览器的页面脚本，在网页的 JS（回溯引擎）里跑，所以这条风险是真实的；缓解全靠 P4.0 的规则：不允许重复里套量词或多选、每条最多两个无上限量词、不超过 256 字节，链接最长 2048 字节，最坏是平方级，约几百万步。规则只在 Go 里校验，`ScriptWith` 写进页面前再校验一次。浏览器扩展不认插件站点（扩展的文件由 `go generate` 生成、随版本发布，装不进用户的包），设置页的说明写的是「浏览器扩展不支持插件网站，请在看懂内置的浏览器里使用」。同意的网站一变，内置浏览器会用新脚本重新载入当前页面（主窗口显示别的页时等浏览器页再显示，开着设置不算；nsgo#840 之后不再重建 web view，后退的历史保留），页面里没保存的状态（比如播放进度、填了一半的表单）会丢。
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

1. **本轮只做 P0（0.1、0.2、0.3）和 P1 的 1.1–1.4**：1.1 是设置 › 插件页、安装和 glossary、ai-service 两种 kind；1.2 是 `segment.Annotated`；1.3 是 dictionary；1.4 是 wordlist 和逐词标注。理由：这几项不执行第三方代码，对学习者价值最高，能单独发布，也不依赖还没决定的根密钥保管。P2、P3、P4、P5 推迟。（之后 P2 的 2.0–2.5 在 `feat/plugins-tasks` 做了，见第 15 节。）
2. **P1.5（随 app 分发官方词典包）跳过，词典包由用户自己安装**。理由：JMdict、CC-CEDICT 是 CC-BY-SA，随包分发要先过法务；官方包还会让安装包明显变大。设置 › 插件页和查词卡的提示引导用户「从文件安装…」。
3. **本轮实现的 kind 只有 glossary、ai-service、dictionary、wordlist**。其余 kind 解析器认得，`Validate` 用 `ErrUnsupportedKind` 拒收，到各自那一期再放开（第 3.1、4 节）。理由：不给还没做的阶段留死代码；旧版本遇到新 kind 时也有明确、可本地化的报错。（P2 之后又放开了 task、translate-style、export-template，P3a 放开了 script，P4 放开了 site，现在九种 kind 都能安装，没有被拒收的；`ErrUnsupportedKind` 留给以后新加的 kind，见第 9 节。）
4. **P3 才有的东西本轮一律不写**：不导入 `nsgo/tsx`，不定义 `Grant`、`Perm`、`Credential`，不做 health.json、in-flight 标记和 audit.log。`state.json` 的每一项只有 `{id, hash, enabled}`，`Grant`、`Signer`、`OldHash`、`Budget` 到 P3 再加（5.1 节）。理由：同上，避免没有调用方的代码；`State.Format` 保留，P3 升格式时旧文件按 6.2 节的规则处理。
5. **P1.2 的选择按测量结果定**：在 Ubuntu 编译机上（`ubuild`）量完整 features 的 `ipa.Dict()` 相对 `DictShrink()` 的常驻内存增量（RSS）和首次加载时间。增量不超过 200 MiB、加载不超过 3 s，就在桌面上、只在启用了日语的词典包或词表包时按需加载完整词典；否则用 E1 的退路：Annotated 不出 Base 和 Reading，读音只取自词典包。理由：原形和读音对查词命中率帮助很大，但不能让所有用户为它付内存；手机的分词始终不变。测量数字和最终选择写在第 15 节。
6. **看懂自己的 LLM key 留在 settings.json 原处**，本轮不迁进 `core.Secrets`。理由：与插件系统无关，本轮的数据包也碰不到它；以后是否迁移见 12.3。
7. **不新增任何 Go module**。确实需要时停下来报告，不自行加入 go.mod。（P3a 起由 owner 放开 regexp2 和 pprof 两项，见第 11 节。）
8. **NSGo 的问题已提交上游**：[nsgo-dev/nsgo#759](https://github.com/nsgo-dev/nsgo/issues/759)（tsx 的回溯正则不响应 Interrupt，已复现：200ms 发 Interrupt，10s 后仍在跑）、[nsgo-dev/nsgo#760](https://github.com/nsgo-dev/nsgo/issues/760)（`tsx.Limits` 没有内存上限）、[nsgo-dev/nsgo#761](https://github.com/nsgo-dev/nsgo/issues/761)（`pkg.Contents` 不支持 app 自定义的签名条目）。三者都只影响 P3，本轮不需要绕行。之后：#759、#761 都已在上游关闭，#759 的修复看懂已经在用，#761 看懂已经认包里签的清单（4.3），但 `nsgo publish` 还签不了它，已发布的包仍靠比对 `kandoManifest()`；#760 仍开着（12.1 第 2、3 条）。

### 12.3 仍待 owner 决定

1. **（已定大部分）** 官方根公钥已定，写在 `script.OfficialRoot`（P3a）。是否开放第三方证书按建成的 `yldm-tech/kando-plugins` 定了：发布者开「New publisher」issue 并勾选脚本插件，owner 审一次代码后用离线的插件根给对方的发布密钥签证书，只覆盖 `ai.yldm.kando.plugin.<发布者>.*`，最长 365 天，到期再开 issue 续；`publishers.json` 给这样的发布者记 `"scripts": true`，CI 拒收别的发布者的脚本插件；看懂把它们显示为「已验证」；每个版本的源码（`scripts/<id>/src/`）在 PR 里审核，审核要点含 network 规则必须最窄。目前只有 `kando` 一个发布者能发脚本插件。仍待决定的只有**插件根密钥的保管**：离线介质、备份份数、轮换策略。
2. **（已定一部分）开发者模式的细节**：本机侧载按建议做了（P3a，第 15 节）：设置 › 插件 › 开发者 打开开发者模式后用本机的开发者密钥打包安装，开发者插件带「开发者」徽标，只在这台电脑上运行，不能用 `kando.` 发布者，也不能替换已安装的非开发者插件。仍待决定的：dev 包何时过期；是否做 trust.json 的回退检测（6.4 节的可选加固）。
3. **（已定，P3a 按此实现）P3.0 spike 的放行门槛**：owner 接受约 +20 MiB 的体积和 regexp2、pprof 两个依赖；默认 token 预算随 `llm.chat` 到 P3b 再定。原文：测量已做完（第 15 节），建议是有条件放行，要 owner 拍板的是：能否接受安装包里的可执行文件大 19–21 MiB（约 +37%，削不掉，原因见第 15 节）；能否接受最坏卡一个 500ms 片段、多占约 0.5 GB，再由看门狗停掉插件；P3 要在 `apps/kando/go.mod` 加 `github.com/dlclark/regexp2/v2`（变成直接依赖，用来设 `DefaultMatchTimeout`）和 `github.com/google/pprof`（indirect），这与 12.2 第 7 条「不新增任何 Go module」字面上冲突，版本都是 NSGo 已经选定的；默认每日 token 预算（建议每个插件 50k）。
4. ~~**是否做站点包（P4）**~~：owner 决定做，P4.0、P4.1 已完成（第 15 节）。
5. **（已定大部分）社区索引**（P5）：客户端、签名格式和 `kando plugin index` 已做（第 15 节）；索引有自己的密钥（`script.IndexKey`，私钥由 owner 保管）。owner 已定托管方式：社区插件放在公开仓库 `yldm-tech/kando-plugins`，官网的 `/plugin` 页列出目录。建成的流程（第 15 节「发布社区索引」）：作者开 PR，Validate 检查（布局、发布者归属、许可证、大小、版本号变大，再用看懂自己的校验器检查打好的包），bot 把预览贴成 PR 里的一条评论；维护者（`publishers.json` 的 `maintainers`，即 CODEOWNERS 里的 houko 和 Session500）审核后合并：main 的规则集要求每次改动走 PR、「Validate packs」通过、至少一位审核通过，`packs/` 和 `scripts/` 之外的改动还必须经 code owner 审核；合并到 main 后 Publish 签名并发布。所以代码包审核由这两位维护者负责，数据包也一样要审核后才上架，和 10 节 P5 原定的「数据包限速后即时上架」不同。仍待 owner 决定的只有**限速**：要不要让数据包过了 CI 就自动合并、按发布者限速（那样要把限速写进 `validate.yml`），还是一直都人工审核。
6. **是否把看懂自己的 LLM key 迁进 `core.Secrets`**：与插件无关的加固，作为独立 PR，Linux 上需要保留明文退路。
7. **外部原生工具或 MCP 子进程插件**：本设计否决了。如果以后识别（faster-whisper）、下载器确有需求，需要单独立项，前提是先给 `media.Command` 补上进程组或 Job Object、WaitDelay 和整棵进程树的 kill。顺带一提，这三项的缺失本身就是一个与插件无关的现有风险：任务取消时，yt-dlp 启动的 ffmpeg 孙进程不会被一起杀掉，`runFollowing` 可能卡在 `cmd.Wait`。这一点是读代码推断的，没有复现，建议单独开 issue。
8. **dictionary 包的条目上限**：现在是 64 个（第 4 节，`limitsOf`），Jitendex（2026.10.03.0）整包 477 个条目，装不上；JMdict 和 CC-CEDICT 装得下。数字见第 15 节「P1.3 真实词典的测量结果」。建议改成 1024：给 Jitendex 留一倍余量，1024 条中央目录记录（Jitendex 的 475 条是 37 KiB）也远小于 `openZip` 读目录时的 1 MiB 上限，所以那个上限不用动，只改 `zipHeadersMax` 注释里「最多 64 个条目」的说法和第 4 节讲 `openZip` 的那句。解压后的 1 GiB 和 zip 的 200 MiB 都够（Jitendex 519 MiB、37 MiB）。

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
| P1.6 | 手机端逐词标注：协议 v2 新增 `films.marks`，桌面按需为没打开的影片算标注并缓存，手机的字幕层和卡片画读音、等级线和虚线，存进 `marks.json` 离线可用，设置 › 字幕样式 的总开关 | `feat/plugins-phone-marks` | 已完成，出入见下 |
| P2（2.0–2.5） | `media.ChatWith`；`aiAsk`；task kind 和每日预算；translate-style kind；`saved/targets.go` 和 export-template kind；把术语表、生词本导出成包 | `feat/plugins-tasks` | 已完成 |
| P3.0 | spike：体积、引擎阻塞、in-flight 开销、ReDoS 和大分配、缓解手段、签名命令 | `feat/plugins-script-spike`（只提交本文档） | 已测完，结果见下 |
| P3a | 脚本插件的运行时和安装路径：`plugins/script`（Trust、Verify、预读沙箱、Limits、Sandbox、看门狗、熔断、in-flight 自愈）、script kind 的清单和校验、state.json 的授权、每个插件自己的 host 和 handler、命令贡献、安装确认和详情页的授权、声明式设置、审计日志、开发者模式、`kando plugin validate` 认脚本插件 | `feat/plugins-script` | 已完成，出入见下 |
| P3b | 其余贡献点（查词来源、导出格式、学习导出目标、标注器）、`kando.llm.chat`、`kando.sound.read`、`credentialFetch` 和 `credentials`、吊销列表的每日拉取 | `feat/plugins-script-contrib` | 已完成，出入见下 |
| P3c | 官方参考插件 `kando.ankiconnect`，官方插件的签名和打包工具，吊销列表随每次发布附上（客户端拉取已在 P3b），Windows 上单独跑插件测试 | `feat/plugins-official-anki` | 已完成，出入见下 |
| P4.0 | `core.ValidateVideoSite`、`ValidateVideoSites`、`VideoMatcher` | `feat/plugins-sites` | 已完成 |
| P4.1 | site kind、`pluginSites`（工具栏的「下载视频」和网页按钮）、逐域名同意、站点包任务不借用登录 | `feat/plugins-sites` | 已完成 |
| P5 底座（E14） | `*media.StageError`（失败带步骤和来源，library.json 加 `errorStage`、`errorSource`）、`media.Tools.Engine`/`EngineName`、`Tools.Candidates`，失败卡片说出是哪个翻译来源失败的 | `refactor/media-stage-error` | 已完成，出入见下 |
| P5 社区索引 | `plugins/index`（签名索引的格式、校验、缓存和 ETag、下载）、设置 › 插件 的「浏览插件」、`kando plugin index` | `feat/plugins-community-index` | 客户端和工具已完成，出入见下 |
| P5 目录发布 | `cmd/kando-plugin`（kando-plugins 仓库 CI 用的 validate、preview、index，不依赖 cgo）、`plugins/publish` 和 `plugins/script/signature`、`kando://plugin/install?id=` 链接、目录的更新提示、官网 `/plugin` 页 | `feat/plugins-directory-site` | 已完成，出入见下 |
| P5 翻译来源（E14） | 贡献块 `translators`、`translate`/`candidates` 导出、设置 › 翻译 的「翻译来源」、`media.Tools.Engine` 的插件适配器、⇧E 改走 `Tools.Candidates`、带码的来源失败、官方 DeepL 插件 `kando.deepl` | `feat/plugins-translators` | 已完成，出入见下 |
| P5 E15 | 播放器侧栏的「扩展」标签：`contributes.panels`、`script.Runner.Panel`（`tsx.View` 外包 `core.Catch`，同一个沙箱，失败计入熔断）、面板视图自己的 Host 和 `line.changed`、`film.Extender`、选择存进 `pluginPanel` | `feat/plugins-panels` | 已完成，出入见下 |
| 授权页补齐 | 6.7 的残余风险上授权页、单端点规则的说明、详情的权限四组、安装确认的大小、列表的「需要授权」「离线时不可用」、数据包的审计记录；6.2 的权限依赖改成如实写「没有依赖」 | `feat/plugins-grant-ux` | 已完成，出入见下 |
| P5 其余 | 按需单独立项 | — | 未开始 |

P1.2 的测量结果（Ubuntu 编译机，x86_64，Go 1.27.4，kagome-dict/ipa v1.2.6；每次读 `/proc/self/status` 前先 `runtime.GC()` 和 `debug.FreeOSMemory()`，各在一个新进程里量，量完的测试没有提交）：

| 情形 | 加载时间 | RSS（VmRSS） | 峰值（VmHWM） |
|---|---|---|---|
| 空进程 | — | 8.3 MiB | 8.3 MiB |
| 只读 `DictShrink()`（`Words` 现在的做法） | 0.45 s | 50.1 MiB | 86.4 MiB |
| 读过 `DictShrink()` 后再读 `ipa.Dict()` | 0.60 s | 141.7 MiB（+91.6 MiB） | 195.4 MiB（+109 MiB） |
| 只读 `ipa.Dict()` | 0.59 s | 113.0 MiB | 147.7 MiB |

`ipa.Dict()` 在 `DictShrink()` 之后读时是另读一份（`DictShrink` 只在完整词典先读的情况下复用它），所以增量按第三行算：常驻约 92 MiB、峰值约 109 MiB，加载约 0.6 s，都在 12.2 第 5 条的门槛（200 MiB、3 s）以内。选择：用完整 features 词典，只在桌面、只在查词用到已启用的日语词典包时（P1.4 起还有日语词表包）由 `segment.Annotated` 第一次调用时读入；手机不调 Annotated，分词照旧只用 `DictShrink`。

P1.3 真实词典的测量结果（2026-11-20，Ubuntu 编译机，x86_64，24 核，NVMe，Go 1.27.4，GOGC 默认；用 `apps/kando/plugins/dict/real_test.go` 的 `TestRealDictionary`，它只在设了 `KANDO_DICT_BENCH` 时跑，每本词典在一个新进程里量，例如在 `apps/kando` 下跑 `KANDO_DICT_BENCH=$PWD/JMdict_english.zip go test ./plugins/dict -run TestRealDictionary -v`；`go test` 在包目录里跑测试，所以路径要写绝对路径，或者相对 `apps/kando/plugins/dict` 的路径；词典按 4.1 节的样子打成包：manifest 加一个词典文件；建索引之前先 `runtime.GC()`、`debug.FreeOSMemory()`，再往 `/proc/self/clear_refs` 写 5 把 VmHWM 清回当前值，所以峰值是建索引这一段的；查词是 10 个词（9 个查得到、1 个查不到，日语含按原形查的 `食べた`、`行った`）各查 1000 次，页缓存是热的；词典数据量完就删了，没有提交）：

| 词典（版本） | 文件 | zip 条目（整包） | 解压后 | `Validate` | 建索引 | 键数 | 索引 | 建索引的峰值（VmHWM 增量） | 查词 平均 / p99 / 最慢 |
|---|---|---|---|---|---|---|---|---|---|
| JMdict 英文（yomidevs 2026-11-19，`JMdict_english.zip`） | 14.9 MiB | 55（57） | 164 MiB | 通过，0.35 s | 10.9 s | 897,354 | 61.9 MiB | 107 MiB | 47 µs / 74 µs / 0.4 ms |
| JMdict 英文带例句（同上，`JMdict_english_with_examples.zip`） | 17.3 MiB | 55（57） | 179 MiB | 通过，0.43 s | 13.9 s | 897,354 | 67.4 MiB | 108 MiB | 53 µs / 92 µs / 8.9 ms |
| Jitendex（2026.10.03.0，`jitendex-yomitan.zip`） | 37.0 MiB | 475（477） | 519 MiB | **拒收**：`ErrTooLarge`（too many files） | 40.6 s | 662,355 | 68.4 MiB | 80 MiB | 51 µs / 120 µs / 9.6 ms |
| CC-CEDICT，Yomitan 版（MarvNC 2026-11-19，`CC-CEDICT.zip`） | 7.4 MiB | 22（24） | 125 MiB | 通过，0.16 s | 10.2 s | 406,944 | 26.2 MiB | 48 MiB | 34 µs / 50 µs / 0.2 ms |
| CC-CEDICT 原文（MDBG，`cedict_ts.u8`，`cc-cedict` 格式） | 9.4 MiB | — | — | 通过，6 ms | 0.53 s | 203,497 | 17.2 MiB | 38 MiB | 32 µs / 48 µs / 0.2 ms |

Jitendex 那一行的索引是跳过 `Validate`、直接 `dict.Open` 建的，现在的 app 装不上它。结论：

- 64 个条目的上限：JMdict 和 CC-CEDICT 够，Jitendex 不够（219 个 term bank、251 个 SVG 图、3 个目录条目、2 个 JSON）。至少要 477，改成多少由 owner 定（12.3 第 8 条，建议 1024）。中央目录最大的是 Jitendex 的 37 KiB，`openZip` 的 1 MiB 不用动。zip 的 200 MiB 和解压后的 1 GiB 都够。
- 峰值内存最多约 108 MiB（JMdict），没到要改成外排序的程度；键数离 `keysMax`（约 839 万）很远。打开已经建好的索引约 10 µs，查词平均约 50 µs，最慢的一次不到 10 ms，不用改。
- 建索引慢：Yomitan 格式要 10–41 s，同一份 CC-CEDICT 的原文只要 0.5 s。CPU profile（Jitendex）里 67% 的时间在 `glossText` 和 `contentText`：structured-content 每深一层就把那一层的 `json.RawMessage` 再 `json.Unmarshal` 一遍，同一段 JSON 被反复解析，嵌套越深越慢。建索引在后台、每份内容只建一次，所以不影响查词，但装好 Jitendex 后要等约 40 s 才查得到。没有改，留作后续：把一条释义只解码一次（比如解成 `any` 再遍历）。
- Jitendex 的释义文字有两处不对：`ruby` 里的 `rt`（注音）被当成正文连在汉字后面（`果くだ物もの`），行内的标签之间没有空格（`1-dantransitiveto eat`）。JMdict 的 structured-content 没有这两种情况，读出来是对的。没有改，留作后续：`contentText` 跳过 `rt`、`rp`，并在 `span` 这类行内元素之间补分隔。

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
  - `PluginPresets()` 改成 `pluginPresets(m)`：名称要用调用方的语言，为此 `locales.Messages` 加了 `Language()`（8.2）。插件预设排在全部内置预设之后，下拉框行写成「插件 · 包名 · host」；`ui.Select` 没有带标题的分组，留了 `TODO(nsgo#906)`（E4）。
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
  - 实现时没有拿真实的 JMdict、Jitendex、CC-CEDICT 全量文件测过（测试用小样例）。2026-11-20 补测了，数字和结论见上面「P1.3 真实词典的测量结果」：JMdict 和 CC-CEDICT 装得下，Jitendex 的 477 个条目超过 64 个的上限，装不上，上限由 owner 定（12.3 第 8 条）；建索引的峰值内存最多约 108 MiB，不用改；Yomitan 格式建索引要 10–41 s，Jitendex 的释义里混进了注音，这两处留作后续。
- P1.4（已完成）：wordlist kind 和逐词标注。和正文的出入如下，E2、第 4 节、5.1、5.2、7.4、7.6、8.1–8.4、第 9 节已同步改过：
  - `Mark` 去掉了 `Tip` 和 `Tier`：`core.Span` 没有悬停提示，也没有别的地方读 tier 的 id；加了 `ToneAccent` 等四个常量。计算放在新的非界面包 `plugins/annotate`，它的 `Mark` 和 `subtitle.Mark` 字段相同，app 用类型转换，`film` 照旧不导入插件包。
  - 缓存只留当前影片一份，不是按键的 map；键除了「视频 + 字幕哈希」，还包括说话语言、三个开关、选中词表和中文词典的内容、是否读日语大词典和生词本的内容指纹（不是词数，换一个词也重算），任何一项变了都重算一次；键回到已有标注的键时丢掉正在算的那一份。字幕哈希只含每行的起止时间和原文，不含译文，只在播放器换了字幕切片时重算。重算期间 `CueMarks` 照旧给出旧标注（视频和行数相同、这一行原文没变时），不再核对切片身份；除了 `playing`，`afterChange` 也调 `pluginsMarksFollow`，暂停时改字幕或生词本也会重算。
  - 开关放在 设置 › 字幕外观 的「逐词标注」卡片，默认都关；由 `settings.Window.Marks` 注入，读自己的 `settings.MarksApp`。
  - 已学词用生词本里同一说话语言的词集合判断，不调 `WordIndex`（它每次线性扫一遍）。
  - 日语读音只在启用了日语词典包、或选了日语词表时才有（读大词典的门槛，见 E1）；中文拼音只来自中文词典包。词表只给语言和它的 `language` 相同的行画线，说话语言不知道、文字又看不出语言的行（比如没标语言的英文片）不画线。
  - `lineSpans` 除了词和读音的边界，也在画线标注的两端切开（只在最上面那一份；描边副本不画线，不切）：标注来自 `segment.Annotated`，词来自 `segment.Words`，两者不一定切在同一处（比如只有汉字的日语行，`segment.Words` 按中文切）。
  - 描边模式下，读音让行的排版变了，所以描边副本和撑开大小的那一份也带同样的读音 view；没有读音时副本照旧画 Content，不切 spans。
  - 手机端没有标注（协议没有字段）；P1.6 补上了，见 E2。
  - 词表读不了（`wordlist.ErrEmpty` 或读文件失败）时列在插件页的问题区，同一内容不再重试。
  - 截图场景 `TestScreenshotsPlugins` 加了 `plugins-marks`（日语原文带假名注音和等级色）。Linux 上的 nsgotest 不出图，这一张只在 macOS 的截图任务里画得出来，本次只在 Linux 上断言了标注本身，没有看过实际画面；读音 view 的竖直对齐（`VerticalAlignTextBottom`）和描边副本的模糊滤镜是否作用到 attachment 上，要在 macOS 截图里核对（已在 macOS 截图里看过，见下面「插件截图核对」）。
- P1 收尾（已完成）：等级的标注、卸载、Scan 的速度、手机查词卡的来源、查词卡的两句词典提示（7.4、7.5）和插件页词典分组的截图。和正文的出入如下，E1、E2、6.5、7.1、7.2、7.4、7.5、第 9 节已同步改过：
  - 等级不再改字的颜色，改成在词下面画一条该等级颜色的粗实线；已学词从实线改成虚线，两者都有时是等级颜色的虚线（owner 的决定，E2）。设置里的「按等级着色」改名「按等级画线」，说明写明粗线和虚线（7.4）。NSGo 的 `core.Style` 本来就有下划线的颜色、样式和粗细，三个桌面平台都画，没有缺口。描边模式下线只画在最上面那一份，不带黑色描边；粗细和位置按字号的比例，没有在 macOS 截图里看过实际效果（已在 macOS 截图里看过，见下面「插件截图核对」）。
  - 卸载遇到更新版本的 state.json 时在删除之前就拒绝；目录删掉后 state.json 没存上的，插件页说「已完成，但……」，不说失败。安装后没存上也改用这一句（新的 `pluginsErrStateUnsaved`），原来说的是「无法读写这个插件包」，和实际不符（7.1、7.2）。
  - `Scan` 的哈希缓存放在插件目录的 `.hashes.json`，按 `<kind>/<id>` 存每个文件的名称、大小、修改时间（纳秒）和哈希；`Validate` 本身不变，`Scan` 走内部的 `validate(dir, cache)`（6.5）。
  - 手机查词卡的来源：协议的 `WordEntry` 加了可选的 `Source`，不升版本；名称和 attribution 用桌面的界面语言，因为桌面不知道手机的界面语言；手机缓存的旧查词结果没有来源，照旧不显示（E1）。手机截图场景 `player-word` 带了「由 AI 服务解释」一行。
  - 没做的 P1 缺口：手机端没有逐词标注（P1.6 已补上，见 E2）；真实的 JMdict、Jitendex、CC-CEDICT 全量文件和 64 个条目的上限没有测（2026-11-20 已测，Jitendex 超出上限，等 owner 定上限，见 12.3 第 8 条）；读音 view 的对齐和描边、彩色下划线的实际画面要在 macOS 截图任务里看（已在 macOS 截图里看过，见下面「插件截图核对」）；`ui.Select` 没有分组（`TODO(nsgo#906)`，等上游）。
  - 离线词典的提示只看装没装这种语言的词典包，不看开没开：装了却关掉是用户自己的选择，提示「去装一个」不对。插件目录还没读完时不提示，免得把已装的词典当成没装。提示作为卡片底部单独的一行加一个「打开插件设置」按钮，按了先关卡片再打开设置 › 插件（`LookupDictionaries`）。
  - 评审后的修正：「词典没有这个词」不再算上查词出错的词典；手机查词卡的「由 … 上的 AI 服务解释」改用回答时桌面发来的名字（`WordSource.Desktop`，可选字段，不升协议版本）；`State.Save` 写之前重读磁盘上的格式；拒绝卸载时的提示和 state.json 的说明不再重复；`StateUnsaved` 挪进 `plugins` 包。
- P1.6（已完成）：手机端逐词标注。和正文的出入如下，E2、7.6 已同步改过：
  - 协议加的是一个新调用 `films.marks`，不是在 `PathSubtitles` 的回答里加字段：v1 的字幕路径不经 `Desk`，手机也会经存储中转读字幕（`RelayFilms`），在那里加字段要改中转写的文件；新调用列在 `Hello.Ops` 里，协商规则现成（`v2.go`）。手机只在局域网上问，标注可能要算一会儿，经中转的调用不该等。
  - 每行带原文的哈希，不是整份字幕一个哈希：手机上改过原文的行（`session.Corrections`）只丢那一行，其余照画；标注只取决于这一行的原文（和开关），所以按行核对就够了，序号对不上时哈希也对不上。
  - 桌面缓存的是最近 8 部影片的标注（`pluginsPhoneKept`），按键找；键里的字幕哈希只含原文。手机端的标注没有自己的三个开关，只有一个总开关。
  - 生词本只有当前资料库的，所以别的资料库的影片在手机上没有虚线（E2）。
  - 卡片（竖屏时显示原文的地方）也画标注，不只是画面上的字幕；没有标注的词照旧画那条淡黄色的「可点」细线。
  - 评审后的修正：插件目录和词典没读完时回 `Pending` 而不是空结果；同一部影片同时只算一份，新的调用停掉旧的；说话语言照播放器的键取（按影片的 key 找任务，文件夹里找到的影片是空），和桌面播放器的标注一致；手机 `Follow` 换了原文后重问；`Pending` 的重问隔一秒；原文哈希挪进 `plugins/annotate`（`annotate.LinesHash`）。
  - 和 #306 合并：桌面算标注改成按原文复用（`pluginsMarkJob.had`、`run` 只标 `had` 里没有的原文），当前影片和手机共用；`TestPluginMarksForAPhoneOfAGrowingFilm` 锁住边播边做字幕的影片只标新行。
  - 截图场景只画卡片：手机目标上报不了视频的进度，画面上的字幕在截图里是空的。横屏画面上的标注由 `TestMarksOnThePicture` 断言。
- P4.0（已完成）：`core.ValidateVideoSite` 和 `ValidateVideoSites`（`packages/core/videosites_validate.go`）。和正文的出入如下，E8、8.1 已同步改过：
  - 比正文写的更严：字符集之外，反斜杠后面只允许 `\w`、`\W`、`\d`、`\D` 和转义的标点（`\1` 这类反向引用、Go 当 Unicode 类而 JS 当字母读的 `\p`，以及 `\b`、`\s` 都拒收）；大括号只能是紧跟在被重复的东西后面的 `{n}`、`{n,}`、`{n,m}`（每个数最多三位）；不允许以 `]` 开头的字符类（Go 把 `[]a]` 读成含 `]` 和 `a` 的类，JS 读成空类加 `a`）。
  - 「不允许嵌套量词」按 Go 解析出的语法树判断：被 `*`、`+` 或上限大于 1 的计数重复的部分里，不能再有任何量词（包括 `?`），也不能有多选分支（`(a|aa)*` 和 `(a+)+` 一样会让回溯引擎指数级回溯）；只是字符的多选（`(a|b)+`）会被 Go 合并成字符类，允许。另外每个规则最多两个无上限的量词（`*`、`+`、`{n,}`），三个相邻、能匹配同样字符的无上限量词会让回溯引擎对 2048 字节的链接做十亿级的尝试。
  - `path` 必须以 `^/` 开头、以 `$` 结尾，`value` 必须以 `^` 开头、以 `$` 结尾；`param` 是 1 到 64 个字母、数字、`_`、`.`、`-`；域名是至少两段的小写主机名，不是 IP；`not` 里的主机必须在域名下面且不重复；一组网站的域名不能相同，也不能一个在另一个下面。
  - 只看 `domain`、`not` 和 `videos` 的 `path`、`param`、`value`，不看 `menu`、`rows`、`titles`、`cut`，这几样只有页面脚本和扩展读。
  - 加了 `kcore.VideoMatcher`（`NewVideoMatcher` 先跑 `ValidateVideoSites` 再编译），`Find` 返回链接所在的网站域名和它是不是一个视频；`VideoLink` 改成和它共用 `findVideo`，行为不变（`browse/videos_test.go` 的 `TestVideoLinkFollowsParseLink` 照旧通过）。`VideoLink` 不认 `VideoMatcher` 的网站。
  - 评审后又收紧：按原样逐字扫描规则，不看 Go 化简后的语法树（Go 把 `(a|a)+`、`(\w|\d)+` 化简成单个量词，JS 的回溯引擎却照样指数级回溯），被重复的组里有 `|` 或任何量词就拒收，所以 `(a|b)+` 也拒收；上限超过 16 的计数（`\w{0,999}`）算作无上限；`^`、`$`、`(`、`|` 后面和开头的量词拒收（`^*`、`a$+` 在 Go 合法、在 JS 是语法错误）；没闭合或多余的括号拒收。页面脚本里每个网站各自在 try/catch 里编译正则，编译不了的网站被丢掉，不影响其他网站的按钮；这一点改变了浏览器扩展生成的 `content.js` 和 `sites.js`（重新 `go generate`）。`VideoMatcher.Find` 对 `not` 里的主机不再返回网站，和页面脚本的 `siteOf` 一致。
- P4.1（已完成）：site kind。和正文的出入如下，E8、3.1、7.2、8.4、第 9 节、10、12.1、12.3 已同步改过：
  - 先只接了工具栏的「下载视频」，同一分支上的后续提交补上了网页按钮（视频页上的「下载视频」和缩略图上的 +），做法见 E8：最初页面脚本只在 web view 创建时读一次，同意的网站一变就重建 web view、丢掉历史；NSGo 加了 `InjectedJavaScriptBeforeContentFunc`（nsgo#840）之后，脚本绑定在同一个 web view 上，网站一变就用新脚本重新载入当前页面，后退的历史保留；浏览器页在后台时等它再显示才换脚本，免得后台重新载入的页面不静音地自动播放。网页按钮的询问和加入都在 Go 一侧用同一个匹配器重新判断（`browsePageSite`、`browsePageVideo`，`browse.ParseAskWith`、`ParseAddWith`），页面说什么都不信；插件站点的视频不再要求 `kcore.ParseLink` 认为是单个视频，和工具栏一样由包的规则决定。`TestScriptWithKnowsTheSitesGiven` 锁住页面脚本只含合法、已给出的网站，`TestPluginSitePageButtons` 锁住只有已同意的域名进脚本、Go 一侧的判断，以及网站变了之后还是同一个 web view（视图 id 不变、后退还在）、载入的是用户点到的那一页而不是最初的 source、浏览器页在后台时不换脚本、告诉过网页按钮的状态清空。浏览器扩展生成的文件没有变化（`go generate` 后无差异，`.github/scripts/browser-extension.sh` 通过）。
  - 在同意的域名上，工具栏按包的规则判断哪些页面是视频（其余页面按钮变灰、不当播放列表），这些任务不借用登录。除了内置浏览器，没有任何入口多接受一个原本不接受的链接。
  - 站点包任务的标记是新加的 `core.Job.PluginSite`（包 id，`shelf` 的 `storedJob` 存为 `pluginSite`），不是只在构造 Tools 时判断链接：加入时同意过、之后撤回同意或卸载了包，这个任务照样不借用登录；反过来，CLI、MCP 加的同一个链接不是站点包的任务。除了 `Cookies`，`SiteCookies`（内置浏览器页面的 cookie，目前因 nsgo#112 还没接上）也一起清空。
  - 同意存在 `plugins.PluginState.Sites`，不是 5.1 节草图里 P3 才有的 `Grant.Sites`；`State.Format` 不升，旧文件照读。没有条目的包一个域名都没同意（`State.Allowed`），和「没有条目就是启用」的 `Enabled` 相反；state.json 读不了时也一个都不同意。开关包、换内容不会丢掉已同意的域名；生效的是「已同意 ∩ 当前包列出的域名」。P3 引入 `Grant` 后也没有挪进去：站点包不是脚本插件，同意的域名不经 `Effective` 求交，挪动还要升 `State.Format`，所以留在 `PluginState`，5.1 节的草图已改。
  - 安装确认框的复选框默认不勾；同 id 的旧版本已经同意、新版本仍然列出的域名预先勾上（`pluginsTickedBefore`），用户在框里看得到。
  - 校验错误多了两个：`ErrBadSite`（规则不安全、域名是公共后缀）和 `ErrSiteKnown`（看懂自己认识的网站）。站点包最多 8 个网站，每个最多 16 个链接规则、16 个 `not` 主机。
  - 设置页网站分组下的说明一行只说浏览器扩展不支持插件网站（网页按钮已经支持）。
  - 评审后的修正：插件站点页面只能问和加同一个包的视频（`browsePageTakes`）；包的域名不能和 catalog 里的网站重叠；保存 state.json 失败时不再重建两次 web view（已同意的网站在保存成功后才交给浏览器）；`BrowseScript` 只读，脚本在 `newBrowseUI` 里先建好。
  - 截图场景 `TestScreenshotsPlugins` 加了 `plugins-install-site`（站点包的安装确认，两个域名的复选框，勾了第一个）。Linux 上的 nsgotest 不出图，这一张只在 macOS 的截图任务里画得出来，本次只在 Linux 上跑通了场景本身。

- P3a（已完成）：脚本插件的运行时和安装路径。owner 已定：接受链接 tsx 带来的约 +20 MiB；`github.com/dlclark/regexp2/v2 v2.11.0` 成为 `apps/kando/go.mod` 的直接依赖（设 `DefaultMatchTimeout`），`go mod tidy` 多出的 `github.com/google/pprof` indirect 一并接受；官方根公钥 `lGuPTFQh1ONLAwOz71PTtGYNDCJvNf3cHQXgg7tvZNk=` 写成 `script.OfficialRoot`，它给官方发布密钥签的证书只覆盖 `ai.yldm.kando.plugin.kando.*`；第三方脚本插件暂时只走开发者模式，不发「已验证」证书。和正文的出入如下，各节已同步改过或在此说明：
  - 代码位置：非界面包 `plugins` 加了 `script.go`（script kind 的字段和校验、设置的取值、`RunsOn`）、`perm.go`、`grant.go`、`health.go`（health.json 和 in-flight 标记）、`audit.go`、`values.go`（插件设置的值）；`plugins/script` 是 `script.go`（根、Trust、Verify）、`sandbox.go`（`Limits`、`Sandbox`、`Load`、`Probe`）、`runner.go`（`Runner`：全局串行、看门狗、熔断、in-flight 标记）、`pack.go`（开发者打包）。app 一侧是 `state_plugins_script.go`、`state_plugins_host.go`（handler）、`bind_plugins_script.go`，界面是 `settings/plugins_script.go`；共享文件只动了 `keys.go`（`shortcuts` 里加 `a.pluginKeys(cx)`，`groupTitle` 加 `plugin:<id>` 的分支），插件块自己的 `state_plugins.go` 接上了钩子。
  - 贡献点只做了 `contributes.commands`。其余贡献块（`exporters` 等）和 `credentials` 不在 `Contributes`、`Manifest` 里定义，严格解码按未知键拒收，到 P3b 再加；`kando.llm.chat`、`kando.sound.read`、`kando.glossary.suggest`、`kando.candidates.offer`、`kando.net.credentialFetch` 这一版没有 handler，申请它们的插件以 `ErrUnknownPerm` 拒收。`kando.llm.chat` 要等 P2.0 的 `media.ChatWith`，main 上还没有。
  - `kando.settings.get` 是 host handler 的名字，tsx 的 Access 要求它出现在包签名的 `capabilities.host` 里，所以插件照样要声明它；它不需要用户授予（`Perm.Free`），声明了就一直生效，详情页不给它开关。
  - state.json：`PluginState` 加了 `OldHash`、`Signer`、`Grant`（`omitzero`），`StateFormat` 升到 2，但 `Save` 只在有一项带这些字段时才写 2，没有脚本插件的文件照旧写 1，旧版还能读。脚本插件的 `llm.chat` 预算到 P3b；P2 的 task 包已经用同一个 `Budget` 字段存每日预算，它不算格式 2 的字段（旧版读到会忽略），所以只有 task 包的文件照旧写 1。脚本插件没有条目就是停用（`State.EnabledPack`），数据包照旧没有条目就是启用。
  - 更新时的授权（6.2）：拿新版本的声明和**旧版本的声明**比，不是和旧授权比。新声明是旧声明的子集，授权按「旧授权 ∩ 新声明」沿用、开关不变；超出旧声明就仍按「旧授权 ∩ 新声明」运行，并记下 `OldHash`，详情页标出「申请了更多权限」，用户动过任何一项授权就清掉。和旧授权比的话，用户只勾了部分权限时每次更新都会被标成扩大能力。签名者变了按新安装处理（停用、授权清空），并清掉旧签名者的 Storage 分区和宿主代管的 secret。
  - 联网作为一项授权：`Grant.Network` 要么是声明的全部 HTTP 规则，要么没有，详情页一个开关（「联网：POST http://127.0.0.1:8765 /」这样列出规则）。高风险组合（`plugins.Risky`）是「读字幕（`line.current`、`lines.seen`）+ 联网」。
  - health.json 的 Reason 多了 `memory`（看门狗按堆阈值停掉的）。Kando 停用插件时同时把 state.json 的开关关掉，并记 health；用户再打开开关就清掉 health。
  - 看门狗（6.6）：每 100 ms 往引擎投一次心跳，连续 3 次等待超过 250 ms 就停（一次心跳一直没轮到时，过了 250 ms 就算一次，引擎完全卡住也停得下来）；堆是整个 app 的，所以只在一次 `runtime.GC()` 确认这些是活对象之后才算到插件头上（GC 只在堆看起来已经过线时才跑，实测 259 MiB 的堆一次约 39 ms）：调用进行中，堆比这次调用开始时多出 512 MiB；跨调用，这个插件自己的各次调用累计让堆长了 512 MiB 以上，并且 GC 之后的堆仍比它载入时多出 512 MiB（模块数组每次只长一点、每次调用都很短时靠这一条）。跨调用的检查在调用结束时做，从不在调用开始前停掉一个还没跑的插件，app 自己在两次调用之间涨的内存不算到插件头上。宿主替插件等用户回答（`kando.ui.openURL` 的确认框）、或在引擎上做插件系统自己的事（预读、载入）时，暂停正在跑的那次调用的看门狗（`Runner.Pause`）：期限不走，心跳的等待不算；其他来源的引擎卡顿仍会算到正在跑的调用头上，这是剩下的误判风险。决定停用时，看门狗先在自己的 goroutine 上 `Plugin.Interrupt(ErrStopped)`（调用已经结束的就不打断），当场切断正在跑的片段，再到引擎上 Close 并报告；NSGo 缺口：tsx 的 `CallTotal` 是墙钟时间、宿主替插件等的时候停不下来，tsx 也不报告一个运行时的 JS 跑了多久，所以期限、`Paused` 和心跳仍是 Kando 自己的（`runner.go` 的 `TODO(nsgo#925)`（[nsgo-dev/nsgo#925](https://github.com/nsgo-dev/nsgo/issues/925)），nsgo#836 只加了 `CallTotal` 和 `Interrupt`）；另外加了单次调用最长 1 分钟的期限：调用是全局串行的，一个永远不结束的 await 会让所有插件的调用排在它后面。熔断：最近 50 次里 3 次失败就停；带 `code` 的拒绝（`core.Error`，插件故意报告的失败，比如 AnkiConnect 没开）不算。tsx 因为配额停掉运行时后，之后每次调用的错误都带 `ERR_NSH_QUOTA_EXCEEDED`，熔断永远不跳，插件却一直显示为开着；所以调用失败的错误是带资源名（`Resource`）的配额错误时，runner 就按 `memory`（内存配额）或 `failed`（其他）停用（`halted`）；Kando 还没设 `Limits.Memory`，目前实际只有节点配额（`failed`）。插件 throw 或 reject 的错误 tsx 只留码和消息（`tsx/view.go:232-245` 的 `rejection`），所以插件自己 reject 同样的码不会被当成停了。最早的做法是调一个插件不导出的名字来探测，但 `Plugin.Call` 查名字时读的是插件可以改的导出对象：插件在上面挂一个 getter，它就在引擎上跑，不受片段预算和看门狗约束，throw 会让 runner 的收尾半途中断、之后所有插件的调用都排着不动，死循环会卡死整个 app（`TestScriptRunnerRunsNoJSOnAFailure`）。NSGo 缺口：`tsx.Plugin` 只能从 Call 的错误看出运行时停没停、为什么停，代码里留了 `TODO(nsgo#925)`（[nsgo-dev/nsgo#925](https://github.com/nsgo-dev/nsgo/issues/925)）；两次调用之间（比如在定时器里）被 tsx 停掉的插件，要等下一次调用失败才停用。这只对 async 函数有效：同步 `throw` 带的 `code` tsx 不转成 `core.Error`（见下面的 NSGo 缺口）。
  - 预读（4.3）在安装确认前做一次，之后每次载入前再做一次（包目录里的清单可以被改，签名管不到它）。「用到的 host handler 都已声明、声明的都用到」的双向检查宿主不做：代码里调哪些 handler 不能静态确定；未声明的本来就被 Grant 和 handler 的 `Effective` 检查挡住。改成目录 CI 的提醒：`kando-plugin preview` 读签名包的代码文本，列出写明了名字却没声明的调用、声明了却从没调过的权限、要等的三个 handler 没传 `timeout` 的调用，不让包失败（4.3，`hostNotes`）。`kandoManifest` 必须是普通函数（不是 async），预读的 Host 是 nil；它返回的键包括 `optional`（`signedKeys`），4.3 原来漏写了这一项。
  - 来源（`Pack.Source`）只由证书链和 id 的发布者段推出，从不读清单的 `author`：官方根下、发布者段是 `kando`、证书范围是 `ai.yldm.kando.plugin.kando.*` 的才是「官方」（徽标「官方」，安装确认「由看懂发布 · 已验证签名」）；官方根给别的发布者签的证书下的插件是「已验证」（徽标「已验证」，安装确认「由 <发布者段> 发布 · 看懂已验证」）；开发者密钥下的是「开发者」。`kando.` 的插件只认范围是整个 `kando.*` 的官方证书，点名单个 `kando.` id 的证书也拒收。截图场景 `plugins-script-install`（第三方、已验证）和 `plugins-script-install-official`（`kando.ankiconnect`）各一张。
  - 证书范围：盖住 App ID 的证书模式只能是 `ai.yldm.kando.plugin.<发布者>.*` 或完整 App ID，`*` 和 `ai.yldm.kando.plugin.*` 即使来自官方根也拒收（`script.ErrScope`）；发布者 `kando` 只接受官方根。
  - 开发者模式（6.4）：开发者密钥是一把 Ed25519 密钥，既当根也当发布密钥，私钥存在 `core.Secrets`（账号 `plugins.developer-key`），开关存在 settings.json 的 `pluginDeveloper`。「打包并安装文件夹…」读文件夹里的 `kando-plugin.json`（不用写 `script`，打包时填上）和入口 `plugin.tsx`，用 `bundler.Compile` 编译，证书在打包时现签，只覆盖这个插件 id 的发布者段，build 号取当前的 Unix 秒数；不读 `nsgo.yaml`。开发者插件不能用 `kando.` 发布者，也不能替换已安装的非开发者插件。
  - `kando plugin validate`：脚本插件在同样的 Stage 之后，按官方根和插件文件夹里 trust.json 的吊销、build 记录（只读）验签，输出来源和签名者；开发者密钥只在应用的 `core.Secrets` 里，所以开发者插件在命令行里报「不是由看懂信任的密钥签名」；`kandoManifest` 的比对要跑沙箱，只在应用里做。`list` 对脚本插件只按条目的开关和哈希判断是否启用，不验签。
  - 命令（E9）：插件的按键必须带 `mod`、`ctrl` 或 `alt`，键是字母、数字或 F1–F12；和看懂的任何一行（不论 `When`）或排在前面的插件撞键时清空按键，详情页列出这些命令。`when` 只有 `always` 和 `film`（有影片打开时）。`groupTitle` 没有 app，插件名经进程级的缓存 `pluginsNames` 取。
  - 插件设置的值存在 `plugins/.settings/<id>.json`，不是 `plugins/<id>/settings.json`：包目录里多一个文件会让 `Validate` 拒收、哈希改变。P3a 画了五种类型，其中 secret 存在 `pluginhost.<id>.<signer>` 分区、`kando.settings.get` 从不返回它，但直到 P3b 也没有任何 handler 用它：`credentialFetch` 只读凭据（`credential.<id>`），用户存下的 secret 设置永远用不上。所以后来改成 `Validate` 拒收 `secret` 设置，原因里指向 `credentials`，详情页、`Resolve` 和 `SecretKeys` 里 secret 设置的分支一并删掉，官方插件都没有用它。设置现在是四种类型，最多 16 项，text 最长 1–1000 字（默认 200），choice 最多 32 个选项。
  - 插件看到的影片 id 是 `f_` 加 `HMAC(HMAC(filmKey, id + 签名者), 视频路径)` 的前 16 位 hex，`filmKey` 是插件文件夹里 `.filmkey` 的 32 个随机字节，第一次有脚本插件时生成。`kando.lines.seen` 的视界是调色板记的最远播放点，再加上正在播放的这一句。
  - `kando.toast` 用 NSGo 的 `ui.Toaster`，画在主窗口上（`pluginsOverlay`），标题是插件名；`kando.ui.openURL` 只收 https，每次用系统提示框确认，同一时刻只问一个，结果记进 audit.log。每个插件每分钟最多 60 次 host 调用（错误码 `quota`），同时启用的脚本插件最多 12 个。
  - audit.log 写到 512 KiB 时挪成 `audit.log.1`；清洗函数由调用方传进来（`feedbackScrubber().Clean`），`plugins` 不导入界面包。
  - 吊销列表的每日拉取（6.4）没有做，P3b 补上了；`Trust.Accept` 在插件第一次载入成功后调用。
  - 插件 manifest 的 `kando` 写成 `>=MAJOR.MINOR.PATCH`，安装时和 `appVersion` 比，源码编译（版本不是三段数字）不拦。脚本包形状：zip 和解压后都不超过 16 MiB、最多 8 个文件，`.nsgopkg` 不超过 8 MiB。
  - 插件页里脚本插件一组排在最前；来源徽标是「官方」「开发者」，验签没过的插件开关灰掉，原因写在行内。
  - 测试：`plugins/script` 的测试自己起一个带 tree 的引擎（`core.NewRoot` 加 `core.NewTree`；`tsx.Load` 要求作用域挂在 tree 上，没有时返回 `tsx.ErrNoTree`，原来的空指针已由 [nsgo-dev/nsgo#838](https://github.com/nsgo-dev/nsgo/issues/838) 修掉），用现场生成的根签包；spike 的三类敌意夹具都在（n=32 的正则 0.5 s 内失败、setTimeout 分片的 CPU 和内存插件 2 s 内被停、模块数组反复增长在过阈值后停用），`-race` 下时间放宽 5 倍。AST 不变量测试扫全仓：没有 `tsx.Sandboxed`，没有 `tsx.View`（这一期插件不画界面），`tsx.Load` 只在 `plugins/script` 的非测试代码里、第三个参数是 `Sandbox(...)` 或 `probeSandbox(...)`。app 的测试把测试密钥当官方根（`pluginsScripts.official`），用 `script.PackFolder` 签插件。截图场景是 `plugins-script-install` 和 `plugins-script-details`，在 Linux 上只跑了无头断言，没出图；Windows 上的插件运行时测试还没在 CI 上跑过。
  - 评审后的修正：改授权时从 state.json 条目出发，签名者变了就清空授权，开关只在条目的哈希就是当前内容时保留（`State.GrantEntry`）；第一次验签读到的 health.json、in-flight 标记和影片密钥，即使被后一次验签取代也照样生效；每个验过签的插件（不论是否在运行）都读出设置的值和 secret 是否已存，值没读出来之前不写；预读拿插件文件里**未清洗**的清单（`Verified.Manifest`）和 `kandoManifest()` 比，`Validate` 去掉的首尾空白、零宽字符不会让正常插件被判为篡改；来源徽标随验签结果重建；卸载和换签名者时用 state.json 记的签名者清分区（插件现在验不过签时 `Pack.Signer` 是空的）；纯函数挪进了 `plugins`（`Grant.Key`、`Grant.AuditText`、`RulesText`、`State.GrantEntry`、`InstallEntry`、`FilmKey`、`FilmID`、`LineValue`、`Values.Clone`）。
  - 和 P1 收尾、P4.1 合到一起后：`PluginState` 同时有站点包的 `Sites`（格式 1 就有）和脚本插件的 `OldHash`、`Signer`、`Grant`（格式 2）；`Save` 照 P1 收尾的做法先重读磁盘上的格式，磁盘上比 2 新就拒写，没有脚本插件的条目时仍写格式 1。`Scan` 的哈希缓存对脚本插件同样适用：缓存命中时不再重读包，但包的 SHA-256 每次验签（`script.Verify`）都会和清单比对，包被改过、修改时间变了就重新哈希并按 `ErrTampered` 拒收。
  - NSGo 缺口：同步 `throw` 丢掉 `code` 已提交为 [nsgo-dev/nsgo#847](https://github.com/nsgo-dev/nsgo/issues/847)，上游已修（NSGo 13543c6ca5f8 起，Kando 从 729f238f5678 起就已带上，只是 TODO 当时没去掉；同步导出抛出的异常和 promise 的拒绝一样经 `rejection` 转成带 code 的 `core.Error`，`tsx/view.go:173-185`），熔断那里的 `TODO(nsgo#847)` 已去掉，`TestScriptBreaker` 同时锁住 async 拒绝和同步 throw 的 code 都不计入熔断。v0.37.0 及更早的看懂用的是修好之前的 NSGo，所以要在这些版本上报 code 的插件仍写 async 导出。原记录：同步导出函数 `throw` 的错误带了 `code` 时，`Plugin.Call` 不像 promise 的拒绝那样把它转成 `core.Error`：`tsx/view.go:142-150` 的 recover 直接把 goja 的异常交给 `done`，只有 `onReject`（`view.go:166-169`）经过 `rejection`（`view.go:182-197`）。插件因此只能在 async 函数里用 `code` 报告预期内的失败。

- P3b（已完成）：其余贡献点、三个宿主能力、凭据和吊销列表的每日拉取。和正文的出入如下，2.2、4.2、5.3、6.2–6.4 已同步改过：
  - 代码位置：非界面包 `plugins` 加了 `contrib.go`（四种贡献块、`Credential`、`GrantedCredential`、`Services` 表、`PluginHeader`）和 `contribvalues.go`（`CueValues`、`Exported`、`Number`）；`lookup/plugin.go` 的 `FromPlugin` 读查词来源的回答，`plugins/annotate/plugin.go` 的 `FromPlugin`、`Merge` 读标注器的回答；app 一侧是 `state_plugins_calls.go`（三个 handler）、`state_plugins_contrib.go`（贡献点的调用）、`state_plugins_revoke.go`（吊销列表），共享文件只在各自块的接点加调用：`vocabResolve` 一行、`ExportSaved` 一个分支、`ExportTargets` 一行、`subexport` 的格式列表和写文件、`pluginsMarksFollow` 在算完看懂自己的标注后接标注器。
  - 查词来源实现成 `lookup.Dictionary`，接在 `Ask.Dicts` 里数据包词典的后面，所以顺序是「数据包词典 → 插件来源 → AI → 别的语言的词典」；来源的 id 是 `plugin:<插件 id>:<来源 id>`，查词卡说「来自插件「<插件名>」的「<来源名>」」（`vocabLookupSourcePlugin`），不称它为词典；插件没查到而 AI 答了时，只有数据包词典会被说成「没有这个词」。手机的 `WordSource` 加了可选的 `SourcePlugin` 种类和 `Plugin` 字段（不升协议版本，旧手机对不认识的种类不显示来源），手机卡片说同一句（`playerWordSourcePlugin`）。来源的 `languages` 和词典一样，词的语言不知道时都问。`LookUp` 不带 ctx，所以每次最多等 15 s，到时间就问下一个来源，那次调用照常跑完。手机查词走同一条 `vocabResolve`，所以插件来源也回答手机。
  - 导出格式的 id 是 `plugin:<插件 id>:<格式 id>`，存进 settings.json 的 `subtitleExportFormat`；`subexport.Choice.Clean` 原样保留这种值，插件不在运行时 `SubexportChoice` 按 SRT 给出。两个按钮（导出到影片文件夹、另存为）都可以用插件格式，文件名是 `<视频名>.<内容>.<extension>`；等插件回答期间不能再开第二次导出。
  - 学习导出目标在收藏句子的下拉框里和模板包排在一起，id 同样是 `plugin:<插件 id>:<目标 id>`；选了它时「导出」不弹文件夹选择，直接把句子和生词（按 `takes`）交给插件，状态行说「<名称>：已发送 N 条，跳过 M 条」或失败。交给插件的句子和生词不要求插件申请 `kando.saved.read`：这是用户自己按的导出，和导出到文件一样。
  - 标注器在看懂自己的标注算完之后、发布之前，在引擎 goroutine 上逐个插件、每批不超过 50 行地调，`stop` 一置位就在下一批前停；标注的键（`pluginsMarkKey.annotators`）包括插件 id、内容哈希和标注器 id，插件载入或停用后会重算。标注器的 `languages` 要求影片的说话语言已知且相同；不写 `languages` 的标注所有影片。插件标注器开着即生效，没有放进「逐词标注」卡片的开关（插件开关就是它的开关）。手机的标注（#318 的 `pluginsMarksFor`）也经过同一个 `pluginsAnnotate`，所以插件标注器也给配对的手机画；为了不让手机先拿到缺了插件标注的结果，开着脚本插件时 `pluginsMarksReady` 要等它们验完签、载入完。
  - 贡献点收到字幕或收藏的文字（查词来源、导出格式、学习导出目标、标注器）也算读字幕：`plugins.Risky` 改成看清单和授权，`ReadsLines` 或 `lines.current`、`lines.seen`、`llm.chat` 之一，再加上联网或 `credentialFetch`，就标高风险。
  - `kando.llm.chat` 和 `credentialFetch` 离线时从 `Effective` 里拿掉（插件会按新授权重新载入），在线时 `llm.chat` 再过 `paletteTranslator` 的门控。预算复用 task 包的 `usage.json` 和 `State.Budget`；脚本插件改预算走 `State.GrantEntry`，不会让一份没开过的内容被打开；更新时（`InstallEntry`）预算跟着授权保留。
  - 熔断不计 `ErrHostTimeout`：JS 里超时的 `host.call` 带 `ETIMEDOUT` 码，没接住时 `rejection` 把它转成带码的 `core.Error`，`Runner.count` 本来就不计带码的失败，所以不用改 runner，只加了测试（`TestScriptHostTimeoutIsNotAFailure`）。这对所有 host call 的超时都成立，不只 `llm.chat`。
  - 宿主侧的等待（`llm.chat`、`sound.read`、`credentialFetch`、`openURL` 的确认框）期间暂停这次调用的看门狗，否则 1 分钟的调用期限会在 120 s 的 LLM 请求里先到；只在正在跑的调用就是这个插件的调用时才暂停（`Runner.PauseFor`），插件在自己的调用结束后从定时器里发的 host call 不会暂停别的插件的调用。每次调用被宿主等待的时间合计最多 5 分钟（`Watch.Paused`），超过就按 `slow` 停掉插件：一个循环等 host call 的调用不能永远占着全局串行的 runner。用户把 `openURL` 的确认框开着超过这个时间也算在内。插件这边 `host.call` 的 150 s `timeout` 只能由插件自己传（JS 不传时的 30 s 是 NSGo 的 `HostCallTimeout`，宿主改不了），所以它是写给作者的规则（5.3），目录 CI 的 `kando-plugin preview` 对没传的调用写一条提醒（4.3）。
  - `checkSigned` 比较签名能力时不看凭据：凭据是看懂代发的，不是包的能力；`kandoManifest()` 的比对（`signedKeys`）加了 `credentials`，`Probe` 检查所有贡献点的导出函数（`Manifest.ExportsNeeded`）。
  - 插件函数的返回值（`Plugin.Call` 的结果）不经 JSON 往返，tsx 把 JS 的整数给成 `int64`；host call 的参数经 `core.hostValue` 的 JSON 往返，是 `float64`。读返回值的地方（`Exported`、标注的偏移）用 `plugins.Number` 两种都认。
  - 吊销列表：`RevocationsURL` 是 Go 常量（`nsgo.yaml` 没有放自定义键的地方，只在 `updates` 旁加了注释）；测试把根换成测试密钥时（`pluginsScripts.official`）不去拉正式地址，只拉测试给的地址。列表还没有发布，发布流程留在 P3c。
  - `kando.glossary.suggest`、`kando.candidates.offer` 没有做，原因见 5.3。后来 `kando.glossary.suggest` 在「术语建议」一条里补上了（`feat/plugins-glossary-suggest`），`kando.candidates.offer` 仍然没有做。
  - 导出格式的标签页：`ui.Tabs` 的选项是切片，不能绑定（NSGo `ui/tabs.go:30`，标签、位置信号和列数都在建的时候按它做好，`ui/tabs.go:72-138`），选项变了只能重建、丢掉焦点；没有插件格式时看懂自己的三个格式不进 `core.Dynamic`，有插件格式时才按格式集合重建。nsgo#848 只给 `ui.Select` 加了 `OptionsFunc`，`ui.Tabs` 在 f377daba0a2a 仍然没有绑定形式，所以这里照旧，代码里改成 `TODO(nsgo#922)`（[nsgo-dev/nsgo#922](https://github.com/nsgo-dev/nsgo/issues/922)），等上游修好 nsgo#922 后改成绑定。
  - 详情页里插件和看懂写好的文字（权限、添加的功能、密钥发往哪里）在一个按界面语言重建的区域里，切换语言后跟着变，平时不重建，输入到一半的密钥不丢。
  - 一个插件格式的导出在插件没在运行时按 SRT 显示，但改别的选项时不会把记住的插件格式改成 SRT，插件回来后还是它。收藏导出到插件时按「取消」显示「已停止」，插件之后的回答丢掉。
  - 纯函数放进了非界面包：`script.FetchRevocations`、`RevocationsChecked`、`SetRevocationsChecked`、`plugins.Manifest.SecretKeys`；「添加的功能」的文字是 `settings.PluginAdds`，和 `settings.PluginDetail` 放在一起（要用 catalog，`plugins` 不导入 `locales`）。
  - 测试：`plugins` 的清单和凭据规则（`contrib_test.go`）、`plugins/script` 的 Probe 和 host 超时、`annotate`/`lookup` 的回答解析，app 层 `plugins_contrib_test.go`：四个贡献点各一个端到端测试；`llm.chat` 的回答不含地址和 key、用量计入预算和 audit.log、超预算拒绝、超时不熔断；`sound.read` 用假 ffmpeg 切、没有 ffmpeg 拒绝；`credentialFetch` 没存密钥拒绝、只在授权的 host/method/path 注入、响应头只留 Content-Type、不跟随重定向、插件的 `Secrets` 读不到密钥、插件不能自己设凭据的头或 Cookie、离线拒绝；详情页的密钥字段和「只会发送到」一行；没有安全存储时停用并说明；吊销列表 404 照常、吊销后关闭并显示「已被撤回」、过期只暂停联网的非官方插件。截图场景 `plugins-script-lookup`、`plugins-script-subtitle-format`、`plugins-script-exporter`、`plugins-script-credential`、`plugins-script-marks` 只在 Linux 上跑了无头断言，没有出图。

- 术语建议（`feat/plugins-glossary-suggest`，已完成）：`kando.glossary.suggest`。#287 之后播放器的「术语」标签已经有「提议一个术语、由用户取舍」的地方（`state_glossary_suggest.go`），所以按 5.3、6.2、7.4 接上了。和正文的出入如下，5.3、6.2、7.4 已同步改过：
  - 没有复用 `core.Proof`：它是终审对字幕行的改动，不管术语表。复用的是 AI 服务术语建议的那一套：「添加」是 `glossaryChange(correctKindTermAccept, …)`，撤销后重新建议；「不用」写进影片旁建议文件的 `dismissed`。
  - 插件的提议只在内存里，换影片就清空，不写建议文件；只有「不用」会写文件，这样插件下次再提同一个词也不会列出来。「不用」写成功后才从内存里去掉，并让还在路上的那次读建议文件作废，免得旧内容把它带回来。
  - 影片的术语还没读完时提议照收，不回 `unavailable`：插件听到 `film.opened` 就调用是最常见的用法，那时术语多半还没读完，插件也没有「读完了」的事件可等。术语表已有的词由 `TermSuggestions` 在显示时滤掉。
  - 插件被关闭、撤销授权或卸载后，它已经提的词仍留在列表里，直到换影片；只是提议，要用户点「添加」才会进术语表。
  - 上限是每次 20 条、每条原文和译文各 60 字（和 `media.SuggestMax`、`suggestLongest` 一样）、一部影片打开期间合计 60 条；纯函数在 `plugins/suggest.go`（`plugins.Suggested`），handler 在 `state_plugins_host.go`（`pluginsGlossarySuggest`），状态在 `state_glossary_suggest.go`。`film.Glossary` 加了 `TermSuggestionPlugin(cx, source)`，返回提议这一条的插件名（界面语言），AI 服务的建议返回空串。
  - 授权页和详情页的说明是「为正在播放的影片建议术语，由你决定加不加进术语表」（`settingsPluginsPermGlossarySuggest`）；它不读片库、不联网，所以不影响 `plugins.Risky`。
  - 测试：`plugins/suggest_test.go`（形状和上限）、`film/terms_test.go`（插件名那一行）、根目录 `plugins_glossary_test.go`（没授权时被沙箱以 `ERR_ACCESS_DENIED` 拒绝；授权后锁定的、全局的、AI 已建议的和重复的都不收；术语还没读完时照收，读完后影片已有的词不列出；列出插件名；添加、撤销、不用写进 `dismissed` 后再提不收；超出上限、缺译文、不是列表整次 `bad-request`；别的影片 `not-found`；合计 60 条后不再收）。截图场景 `player-terms-plugin`（「术语」标签里一条 AI 建议、一条插件建议）只在 Linux 上跑了无头断言，没有出图。

- P5 社区索引（客户端和工具）：目录的页面、上架和限速、代码包审核不在这个 PR 里（官网上的页面另做），本 PR 没有建任何仓库，也没有上传任何东西。和正文（10 节 P5、6.5）的出入和细节如下：
  - 格式（`plugins/index`，非界面包）：索引文件是 `{"format": "kando-plugins-index", "version": 1, "signer", "signature", "body"}`，`body` 是索引 JSON 的 base64，里面是 `format`、`version`、`generated`（生成时间）和 `entries`。签名是 Ed25519 对 `kando-plugins-index-v1`、一个 0 字节和 body 的 SHA-256 签的，做法和 tsx/pkg 给包、证书、吊销列表签名一样，但上下文不同，所以同一把键签的别的东西不会被当成索引。这份 framing 是 `plugins/index` 自己的（`framed`），只复用 tsx/pkg 的键编码：`pkg.DecodePublicKey`、`DecodeSecretKey`、`EncodeSecretKey`、`KeyID`、`GenerateKey`。上游按 [nsgo-dev/nsgo#868](https://github.com/nsgo-dev/nsgo/issues/868) 导出了文档签名封装（`pkg.SignDocument`、`pkg.Signed.Open`），但它只收以 `app:` 开头的上下文：`kando-plugins-index-v1` 会被拒（`pkg.ErrContext`），换成 `app:kando-plugins-index-v1` 签出来的又是另一份内容，kando.yldm.ai 上已发布的索引和已发布的 Kando 都验不过，所以不换；`TestIndexSignatureIsStable` 用固定键的签名、`testdata/live-index.json`（下载的线上索引）和这两点把它钉住。要换只能用新的上下文、把索引的 `version` 升到 2，旧版会报 `ErrVersion` 直到更新。格式和版本在签名外也写一份，好让旧版在验签前就认出新格式（`ErrVersion`）。验签之后用 `DisallowUnknownFields` 严格解码，再逐条校验。
  - 条目：`id`、`kind`、`name`、`summary`（清单的 description）、`version`、`publisher`、`license`、`homepage`、`size`、`sha256`、`url`、`languages`、`minKando`、`source`。校验复用清单的规则（`plugins/check.go` 把 `checkID`、名称和说明的清洗与长度、保留名、版本、许可证、语言标签导出成 `plugins.CheckID` 等）：kind 必须是这个版本能装的；`publisher` 必须等于 id 的第一段；`size` 在 1 字节和这种 kind 的 zip 上限（`plugins.ZipMax`）之间；`sha256` 是 64 位小写 hex；`url` 只认 https（回环地址的 http 也不行，`plugins.CheckHTTPS`），`homepage` 和清单的规则一样（`plugins.CheckHomepage`）；`languages` 最多 16 个、不重复；`minKando` 只有脚本插件能写，是 `MAJOR.MINOR.PATCH`；`source` 只有脚本插件写，而且只能是 `official` 或 `verified`，数据包不写。整份索引最多 4 MiB、5000 条。`kando plugin index` 签名时（`Index.Check`、`Sign`）对条目是严格的：任何一条不对、同一个 id 出现两次，就不签。看懂读索引时（`Parse`）只对文件、签名和索引自己的键严格；条目逐条解码，这个版本读不了的（不认识的键、类型不对）、装不了的（不认识的 kind、文件超过这种 kind 的上限）、不合规则的和重复的 id 都跳过并计数（`Index.Dropped`），其余照常列出，「插件目录」在状态行下面写一句「列表里有 N 个插件需要更新版本的看懂，没有显示」。这样给新版看懂加了新 kind 或新字段的索引，旧版照样能用，不会因为一条就整份失效。
  - 公钥：`script.IndexKey`（`Grk9XL+lf71LM+opo08YYlOWxk0aRWvIw0205NgA9rg=`），写在 `script.OfficialRoot` 旁边，是一把单独的键，和官方根分开：根密钥一直离线，只给发布密钥签证书、签吊销列表，从不签索引。索引密钥的私钥由 owner 保管，签索引时只经环境变量 `KANDO_PLUGINS_INDEX_KEY` 交给 `kando plugin index`。`script` 包的 `TestScriptIndexKeyIsItsOwn` 断言它不等于 `OfficialRoot`。常量为空时「浏览插件」显示「这个版本的看懂没有插件目录」，不发任何请求（`index.ErrNotConfigured`）。
  - 地址：`index.DefaultURL` 是 `https://kando.yldm.ai/plugin/index.json`，包文件在 `https://kando.yldm.ai/plugin/packs/<文件名>`。owner 定的：不用 R2 和子域名，地址在现有官网的域名下；索引和包同在一个主机，所以「重定向只跟同一主机」的规则照样成立。索引和包文件由 `yldm-tech/kando-plugins` 的 CI 签名并发布到那个仓库自己的 GitHub Pages，`kando.yldm.ai/plugin/index.json` 和 `/plugin/packs/*` 由 Cloudflare Worker 从那里转发；`/plugin` 页面本身在本仓库的官网（`apps/site`）。流程见下面「发布社区索引」（本 PR 原先写的是 owner 在本机签名后提交到 `apps/site`，P5 目录发布改成了现在的做法，本机签名留作退路）。应用更新走的是 GitHub Releases 的 appcast（`nsgo.yaml` 的 `release`），和这里无关。
  - 缓存和刷新：索引原样存在插件文件夹的 `.community/index.json`，旁边的 `index-meta.json` 记着地址、ETag 和上次询问的时间。缓存只属于它来自的那个地址：地址变了，`Cached` 返回 `ErrOtherAddress`，不显示、也不拿来比新旧。只在用户打开「浏览插件」时读缓存；缓存读不出来（文件没了、换了公钥、别的地址的）、距上次询问满 24 小时（`index.RefreshEvery`）、或者没有缓存，就马上问一次，「刷新」随时可以问；缓存在却读不出来时，原因显示在状态行，直到新的索引到来。请求带 `If-None-Match`，304 时沿用缓存；新索引的 `generated` 早于缓存的拒收（`ErrRollback`），防止被攻破的主机把撤下的条目换回来；验签、格式、条目不对时保留缓存，状态行写原因。离线模式下不发任何请求，显示缓存和它的日期（离线这一句排在之前联网时的错误前面），「刷新」和「安装」都灰掉；正在进行的询问和下载在切到离线时取消（`afterChange` 里的 `pluginsIndexFollow`），收起「插件目录」也会取消，取消之后才回来的结果一律丢掉（每次询问和下载各带一个代数），所以不会在离线时弹出安装确认。启动时不问：插件页不打开就不会有这个请求，所以没有加 `areaHooks`。所有请求只认 https，重定向只跟同一主机的 https，最多 5 次（`ErrRedirect`）。
  - 安装：下载不占插件页的 busy：下载期间其他包照样能开关、卸载、从文件安装，那一行显示「正在下载…」和「取消」；同一时刻只下载一个。下载完了，等没有别的安装和确认框时，才在 busy 下 Stage 和验签、打开确认框。先下载到临时文件夹，最多读到条目的 `size` 多一个字节，大小和 SHA-256 都要等于条目的（`ErrTooLarge`、`ErrChecksum`），然后走和「从文件安装…」同一条路径：`plugins.Stage`（里面是 `Validate`）、脚本插件的 `script.Verify`、`pluginsAskStaged` 打开安装确认。另外查两件事：读出的包的 id、kind、version 必须等于条目写的；脚本插件验签得出的来源必须等于条目的 `source`（都是 `ErrNotListed`）。下载和这些检查的失败也显示在确认框的失败视图里。确认框的来源一行写「来自看懂的插件目录 · 未签名」（数据包本身仍没有签名，只有索引签了名），脚本插件在证书来源下面多一行「从看懂的插件目录下载」。授权规则一点没变：task 包和脚本插件装好是关的，脚本插件不授予任何权限，更新时按 `plugins.InstallEntry` 的子集规则。
  - 更新：已装的版本比索引里的旧时（`index.Newer`：按点分段比，数字段按数值比，带 `-` 后缀的比不带的旧，`+` 之后不比），那一行显示已装的版本和「更新」，点了走同一条安装路径，确认框写明替换的版本。不自动更新。需要更新版本看懂的脚本插件（`minKando`）显示「需要看懂 x.y.z」，不能装。
  - 界面：设置 › 插件 顶部的按钮行多了「浏览插件…」（目录展开时灰掉），展开后在已安装列表上面显示「插件目录」：标题旁边是「刷新」和「收起插件目录」、状态行、读不了的条目数、搜索框（按 id、发布者和各种语言的名称、说明找）、种类下拉框，最多列 50 条，更多时下面写一句。代码是 `settings/plugins_index.go`、`state_plugins_index.go`、`bind_plugins_index.go`；共享文件只动了 `state_plugins.go`（`pluginsUI.index`、`pluginsPending.listed`、`newPluginsUI`）、`bind_plugins.go`（`PluginInstall` 的 `Listed` 和错误映射各一行）、`settings/plugins.go`（`PluginsApp` 嵌入 `PluginIndexApp`、按钮、区块、确认框的来源）。
  - 设置 › 隐私 的「每一步会不会联网」加了「插件目录」一行（`settings/privacy.go`，由用户发起，离线时关闭）。README 五种语言的命令行表格加了 `kando plugin index` 一行；`--base`、`--out` 只有 `index` 子命令认，其他子命令遇到它们按用法错误报错；`kando plugin -- index …` 也认得出 index。环境变量里的私钥对应的公钥不是 `script.IndexKey` 时，命令直接拒绝，免得签出一份所有看懂都拒收的索引（测试用 `indexPublicKey` 换成自己的键）。
  - 没有做：吊销列表不随索引发布（P3c）；`kando plugin index` 不读吊销列表，被吊销的包要先从文件夹里拿掉（CI 工具 `cmd/kando-plugin index` 的 `--revocations` 会拒绝列它，见下一条）。
  - 测试：`plugins/index` 的单元测试（签名被改、没签名、换了键都拒收，格式和每个字段，大小上限，httptest 的 TLS 服务器上的缓存、ETag、304、回滚、重定向和下载的大小与哈希）；`cli` 的 `TestPluginIndex*`；根目录的 `TestPluginIndex*`（打开前不发请求、搜索和种类过滤、安装要经过确认、task 包装好是关的、更新、哈希不符和条目不符拒装、离线只看缓存、每日刷新和 304、伪造的索引、没有公钥、脚本插件装好是关的且来源不符拒装、读不出来的缓存马上重新问并说明原因、切到离线时取消询问和下载且不弹确认框、下载期间别的包照样能开关和取消下载）；`TestIndexDropsWhatItCannotRead` 和 `TestIndexCacheIsPerAddress`。截图场景 `plugins-index`（`TestScreenshotsPluginIndex`）在 Linux 上只跑了无头断言（`TestPluginIndexScene`），没出图。

- P5 目录发布：把上一条的客户端和工具接到社区插件仓库 `yldm-tech/kando-plugins`（那个仓库和它的 CI、Worker 由另一个 PR 建）。和上一条、正文（10 节 P5）的出入和细节如下：
  - CI 工具 `apps/kando/cmd/kando-plugin`：`validate [--json] <包…>`、`preview [--markdown] <包…>`、`index <文件夹> --base <url> [--out <文件>] [--json]`。模块路径是 `kando/apps/kando`，别的仓库不能 `go install`，所以 kando-plugins 的 CI 检出本仓库后在根目录 `go run ./apps/kando/cmd/kando-plugin …`（`go.work` 在根目录）。退出码：0 全部通过，1 有包没通过或命令失败（validate 和 preview 先检查完所有包再失败），2 用法错误。`validate --json` 输出 `{"ok", "packs": [{"path", "ok", "id", "kind", "version", "name", "hash", "source", "signer", "size", "sha256", "code", "error"}]}`，`code` 是稳定的错误类别（`reserved-id`、`untrusted`、`tampered`、`bad-manifest`、`not-found` 等，见 `errorCodes`），给 bot 判断用。`preview` 给 PR 评论写 Markdown：每个包的 id、kind、各语言的名称和说明、版本、大小和 SHA-256、许可证、作者、主页、语言、需要的看懂版本、签名来源、校验结果、添加的功能；脚本插件另列权限（必需和可选）、网络规则（`plugins.RulesText`）、凭据和发往的主机，并用一段 `[!WARNING]` 提醒它运行代码，`plugins.Risky` 成立时再说明它读别人写的文字又能往外发。脚本插件还有一节「Host calls」：读签名包里的代码文本，列出写明名字却没声明的 `host.call`、声明了却没调过的权限、调要等的三个 handler 没传选项的地方（`hostNotes`），只是给审核者的提醒，不影响退出码（4.3）。包里的所有文字和路径都写在代码片段里（反引号数量按内容里最长的一串加一，表格里转义竖线，去掉控制字符，每段最多 300 字），所以名称和说明里的 @提及、#引用、链接、图片和 HTML 都不起作用。`index` 和 `kando plugin index` 一样：私钥只从 `KANDO_PLUGINS_INDEX_KEY` 读，不是 `signature.IndexKey`（即 `script.IndexKey`）的键直接拒绝。validate、preview 和 index 用一个临时的 trust，只信任官方根。`validate` 和 `index` 可选 `--revocations <文件或 https 地址>`：读到的列表交给这个 trust（`publish.Revocations` 读文件或经 `signature.FetchRevocations` 取地址，`publish.Trust` 调 `UpdateRevocations`，解析和拉取就是看懂每天拉列表的那份代码，`script.FetchRevocations` 现在转调它），所以包的 SHA-256 或签它的发布密钥被吊销时，`signature.Verify` 和看懂一样报 `ErrRevoked`，`validate --json` 的 `code` 是 `revoked`，`index` 失败并指出是哪个文件，不写索引。地址回 404 当作没有列表，照旧检查；文件不存在时命令失败（路径是调用方自己写的，不会有「还没发布」的情况）；不是官方根签的列表在检查任何包之前就让命令失败；地址只认 https。不给这个参数时什么都不读，也不联网，所以 CI 要用 `--revocations https://github.com/yldm-tech/Kando/releases/latest/download/kando-plugin-revocations.json`（`script.RevocationsURL`）显式打开。列表过期不影响检查，过期列表里的吊销照样生效。preview 不读列表。
  - 同一份代码：检查和建索引搬进了非界面包 `plugins/publish`（`Check`：文件先复制一份、边复制边算 SHA-256，再对副本 `plugins.Stage`，脚本插件 `signature.Verify`；`IndexKey`；`Index`、`IndexFiles`），`kando plugin validate`、`kando plugin index` 和 CI 工具都调它，不会各写一套。`kando plugin validate` 因此也先复制再检查，行为不变；trust 只在遇到脚本插件时才打开（`publish.Trust`），只检查数据包时不读插件文件夹的 trust。工具的参数里第一个 `--` 之后全部当作路径。
  - 不依赖 cgo 和界面：`plugins/script` 经 `tsx` 引入了 NSGo 的 `base`、`desktop`、`mobile` 和 `tsx/bundler`。验签只要 `tsx/pkg`，所以把它（`OfficialRoot`、`IndexKey`、`AppIDPrefix`、`Roots`、`DefaultRoots`、`TrustDir`、`OpenTrust`、`Verified`、`Verify`、`Publisher` 和五个错误）原样搬进了 `plugins/script/signature`，`script` 以同名常量、类型别名、变量和转发函数重新导出，app 和测试的调用处一行没改；`cli` 和 `publish` 直接用 `signature`。`CGO_ENABLED=0 go build ./apps/kando/cmd/kando-plugin` 能编，`go list -deps` 里没有 cgo 包，也没有 `tsx`、`base`、`ui`、`desktop`、`mobile` 和任何 platform 包；还剩 `nsgo/core`（纯 Go），是 `plugins.Text.In` 用的 `nsgo/i18n` 带进来的：`i18n` 的 `Language`、`Regional` 等函数收 `*core.Scope`（`i18n/i18n.go:13`、`i18n/format.go:10` 导入 `core`），只用 `i18n.Parent` 的包也得编进整个 core。没有绕开，在报告里交给主会话决定是否提 NSGo issue。
  - 链接 `kando://plugin/install?id=<id>`（`index.InstallLink`、`ReadInstallLink`，id 只按 `plugins.CheckID` 的语法查）：打开 设置 › 插件，「插件目录」展开并搜索这个 id。不管缓存是否到期，都先问一次新索引（一分钟内刚问过的除外，`pluginsIndexLinkFresh`），因为缓存里可能是旧版本，它的文件目录已经不再提供，也可能还没有这个包；拿到这份新索引、包目录读完、没有别的下载时（`pluginsIndexWanted`，在 `pluginsIndexFollow` 里跑），列着的包能装或能更新就按新索引的版本和文件开始下载，之后走和「安装」同一条路径（下载校验大小和 SHA-256、Stage、Validate、脚本插件验签、安装确认）；已装或需要更新版本看懂的只显示那一行。离线、或者那次询问失败时，链接一直等着，状态行说明原因（离线或失败），不说「没有」；按「刷新」或重新联网后再问，问到了再处理。新索引里没有这个 id 时在状态行下面用红字写「插件目录里没有 {id}」；id 不合语法写「这个链接没有指明插件」。这两句收起目录、点「查看更新」或下一个链接时消失。任何网页都能打开这种链接，所以它只指明一个包：不在签名索引里的东西不下载，没有确认就不安装。5.4 原说 `kando://` 不能触发安装，已按这条改写：链接最多打开签名索引里那个包的安装确认，自己不安装也不授权；浏览器扩展和 MCP 仍然不能触发安装或授权。
  - 链接的分发：NSGo 把在任何 handler 注册之前到达的链接（例如启动看懂的那个链接）只交给第一个注册的 handler（`core.Scope.OnOpenLinks`，`core/window.go:823`），各块各自注册的话冷启动时的链接会丢给别的块。所以 `areaHooks` 加了 `openLinks`，`areasReady` 只注册一个 handler（`areasOpenLinks`）分给每块；书签小工具的 `kando://add` 从 `browserExtHooks` 的 ready 挪到了这个钩子。Windows 和 Linux 起初没有注册 URL scheme（nsgo#262），链接只在 macOS 上能打开，官网在别的系统上改成写出在看懂里的路径（设置 › 插件 › 浏览插件）和可以复制的 id。nsgo#262 修好后（nsgo-dev/nsgo#890，NSGo 729f238f 起，#335 接上），`nsgo build` 按 `nsgo.yaml` 的 `url_schemes: [kando]` 在 Windows 的 NSIS 安装包里写 `HKCU\Software\Classes\kando`（命令是 `"<exe>" -- "%1"`，卸载时删掉，MSIX 是 `windows.protocol`），在 Linux 的桌面项（压缩包、deb、Arch、AppImage、Flatpak）里列 `x-scheme-handler/kando`、`Exec` 用 `%U`，不用 Kando 自己写任何注册；冷启动时命令行里的链接交给第一个注册的 handler，也就是 `areasOpenLinks`，看懂已经在运行时第二次启动经单实例的通道把链接交给它（`EventOpenLinks`），窗口显示着时提到前面、不发 `OnReopen`，第二个进程退出；窗口被关到后台（隐藏）时 NSGo 不显示它，由各块的 handler 显示（`browserExtOpenLinks`、`pluginsIndexOpenLinks` 都调 `remoteRaise`）。Kando 这边只改了 `cli.OpensWindow`：Windows 以 `-- <链接>` 启动，`--` 后面跟链接或文件时照样开窗口，不当成命令。官网在 macOS、Windows 和 Linux 的浏览器上都显示「在看懂中打开」，手机、平板（含 Android、ChromeOS）和认不出系统的浏览器仍写路径和 id（`opensLinks`）。根目录的 `TestPluginIndexLinkFromTheSystem` 和 `TestAddSheetFromTheSystemsLink` 用 `nsgotest.App.OpenLinks` 走系统送链接的那条路；真实系统上的注册和第二次启动要手动核对：Windows 装 NSIS 包后 `Start-Process 'kando://plugin/install?id=kando.deepl'`（看懂没开和开着各一次），Linux 装 deb 后 `xdg-mime query default x-scheme-handler/kando` 是看懂的桌面项，`xdg-open 'kando://plugin/install?id=kando.deepl'` 同样两次，再从浏览器点官网 `/plugin` 的「在看懂中打开」和书签小工具各一次。
  - 更新提示：启动时（插件块的 ready）在后台读一次插件文件夹里缓存的索引（`pluginsIndexPeek`，只读盘、不联网，读不出来就不管，留给「浏览插件」去说），不算「浏览插件」的那次读取，所以打开目录时仍按每日规则决定要不要问。之后每次读到或问到新索引都重算：已装的包在索引里有更新、而且能在这个版本上运行的（`pluginsIndexUpdatable`），数量显示在设置侧栏「插件」旁边（强调色的小圆角数字，读屏读「N 个更新」），插件页顶部显示「有可用更新（N）」和「查看更新」；点了打开目录，只列有更新的包（搜索和种类过滤清空），上面写「只显示有更新的插件」和「显示全部」。目录正以更新过滤打开时顶部那行不显示。除了原有的「打开目录时、每天最多一次」之外没有任何后台请求，离线照旧。共享文件只动了 `events.go`（`openLinks` 字段和分发）、`state_browserext.go`（挪钩子）、`state_plugins.go`（ready 多读一次缓存、`openLinks`）、`settings/window.go`（`App.PluginIndexUpdates` 和侧栏的数字）、`settings/plugins.go`（顶部那行）。
  - 官网 `/plugin`（`apps/site`，五种语言各一页，预渲染的是加载中的外壳）：水合之后从同源的 `/plugin/index.json` 取索引，在浏览器里不验签（看懂安装前自己验，代码注释里写明），只把各字段当不可信的文字显示：逐字段查类型、形状和长度，不合规则的条目和重复的 id 跳过，主页只认 https，从不用 `dangerouslySetInnerHTML`。有搜索、种类过滤、计数，每张卡片有名称、种类、说明、发布者、版本、大小、许可证、语言、脚本插件的签名来源、主页和「在看懂中打开」（`kando://plugin/install?id=`，在 macOS、Windows 和 Linux 上，见上）；加载中、出错（可重试）、离线（联网后自动重试）、空列表各有状态；「如何贡献」一节链接 `yldm-tech/kando-plugins` 和它的 `CONTRIBUTING.md`。页脚加了链接，sitemap、hreflang、结构化数据和 `llms.txt` 都包含这一页。
  - 测试：`cmd/kando-plugin`（validate 的文本和 JSON、错误码、不信任的根；preview 的各项和代码片段转义；index 的密钥、`--base` 和签出的索引能被 `index.Parse` 读；`TestRevocations` 用测试自己生成的根签列表：吊销包和吊销发布密钥都得到 `revoked` 而旁边的数据包照常通过，空列表和 httptest TLS 服务器上的 404 照常通过，不存在的文件、别的根签的列表和 http 地址失败，index 遇到被吊销的包失败且不写索引）；`plugins/index` 的 `TestInstallLink`；根目录 `TestPluginIndexLink`（链接打开设置并走到安装确认、确认前不安装、先问新索引再说没列着、一分钟内刚问过的不再问、索引后来才有的包、坏链接）、`TestPluginIndexLinkWaits`（离线时等着不下载也不说没有、询问失败时等着并显示失败、刷新后按新索引的版本安装，旧版本的文件已经不提供）和 `TestPluginIndexUpdates`（启动读缓存不联网、侧栏数字和顶部那行、只看更新和显示全部、更新后消失）；截图场景 `plugins-updates`（`TestScreenshotsPluginUpdates`）在 Linux 上只跑了无头断言（`TestPluginUpdatesScene`），没出图。官网的 `test/plugins.test.ts`（解析、校验、主页、名称回退、搜索、过滤）和 `test/plugin-page.test.ts`（构建出的页面）。

**发布社区索引**（社区插件仓库 `yldm-tech/kando-plugins`）：

1. 作者在 kando-plugins 开 PR：数据包是 `packs/<kind>/<id>/` 文件夹（Publish 打成包文件），脚本插件是 `scripts/<id>/` 里签好的 `<id>-<version>.kandoplugin`（每个文件夹只放要列出的那一个版本）和签它用的源码 `src/`，发布者要先由官方根认证（12.3 第 1 条）；官方插件也这样提交，见 P3c 的「签名和发布官方插件」。
2. 那边的 Validate 先用基线分支上的 `tools/packs.py pr` 查仓库自己的规则（布局、发布者归属、许可证、大小、版本号变大；维护者加了 `maintainer-override` 标签时不查 `packs/`、`scripts/` 之外的改动和发布者归属），基线分支上的 `tools/packs.py build` 把 `packs/` 的文件夹打成包文件；再检出本仓库，把 `./apps/kando/cmd/kando-plugin` 编成一个可执行文件，对打好的包跑下面两条（这里写成在本仓库根目录 `go run` 的等价形式）：

```sh
# Checks every pack as Kando installs it (plugins.Stage, a script plugin's signature against Kando's official root and its revocation list, which a 404 leaves out); --json for the bot, exit code 1 when one fails, code "revoked" for a pack the list revokes.
go run ./apps/kando/cmd/kando-plugin validate --json --revocations https://github.com/yldm-tech/Kando/releases/latest/download/kando-plugin-revocations.json <pack files…>
# Markdown for the pull request's comment: what each pack is, adds and asks for; every text of a pack inside code spans.
go run ./apps/kando/cmd/kando-plugin preview --markdown <pack files…>
```

3. bot 把 Validate 的结果和预览贴成 PR 里的一条评论，每次推送都更新；维护者（houko、Session500）看过评论审核后合并，脚本插件还要审源码，含 network 规则是否最窄。main 的规则集要求「Validate packs」通过、至少一位审核通过，`packs/` 和 `scripts/` 之外的改动必须经 code owner 审核（12.3 第 5 条）。
4. 合并到 main 后，Publish 在受保护的 environment 里拿到 `KANDO_PLUGINS_INDEX_KEY`：同样先 `tools/packs.py check`、`build`，再 `tools/packs.py carry`（线上已有的同名文件内容相同就沿用线上的字节，内容不同就停下；已不在新索引里的旧文件再保留 7 天，记在 `carried.json`），用 `kando-plugin validate` 逐个检查，然后签名并部署：

```sh
# The key's secret only in the environment; any key other than script.IndexKey is refused.
KANDO_PLUGINS_INDEX_KEY=<index secret> go run ./apps/kando/cmd/kando-plugin index ./packs --base https://kando.yldm.ai/plugin/packs/ --out out/index.json --revocations https://github.com/yldm-tech/Kando/releases/latest/download/kando-plugin-revocations.json
```

然后把 `index.json` 和 `./packs` 里的文件（按原名）发布到 kando-plugins 自己的 GitHub Pages。
5. Cloudflare Worker 把 `https://kando.yldm.ai/plugin/index.json` 和 `/plugin/packs/*` 转发到那里；`/plugin` 页面是本仓库的官网，取的是同一个 `/plugin/index.json`。

- 私钥只从环境变量 `KANDO_PLUGINS_INDEX_KEY` 读（`go tool nsgo keygen` 打印的 secret），不接受 flag 或文件参数，和 nsgo 的 `NSGO_ROOT_KEY`、`NSGO_RELEASE_KEY` 一样；只给 main 上的部署 job，PR 的 CI 拿不到。
- 文件夹里任何一个包没通过检查、有别的文件、或者同一个 id 出现在两个文件里，命令就失败并指出是哪个文件，不写索引，免得发布者以为列上了的包被悄悄漏掉。脚本插件必须由官方根签的证书覆盖（来源是 official 或 verified），开发者密钥签的插件拒收。
- 每个包的 URL 是 `--base` 加上文件名（按 URL 转义），所以文件名必须保持不变。已经发布的包文件不要原地替换：新版本用新文件名（例如带上版本号），因为还缓存着旧索引的看懂会按旧的 SHA-256 校验它；旧版本的文件由 Publish 的 `carry` 在下架后再保留 7 天，够缓存了旧索引的看懂在每天的刷新之前下载。Cloudflare 上 `plugin/index.json` 的缓存时间设短一些（例如 5 分钟），包文件可以长期缓存。
- 上架规则和 10 节 P5 原定的不同：数据包和脚本插件都要维护者审核、合并后才上架，没有限速（要不要改成数据包限速后自动上架仍待 owner 决定，12.3 第 5 条）；上架之后安装仍然不授予任何能力。从索引里撤下一个包只是不再列出，已经装了的不受影响；真正要停掉一个脚本插件要靠吊销列表（6.4）。
- 每次重新生成的索引 `generated` 都更新，看懂拒收比缓存旧的索引，所以不要重新部署旧的 `index.json`（例如回滚 kando-plugins 的提交时，要重新签一份）。
- 退路（CI 或 Worker 不可用时，owner 在自己的机器上）：用同一个命令在本机签（`kando plugin index ./packs --base https://kando.yldm.ai/plugin/packs/ --out index.json`，或上面的 `go run`），把 `index.json` 和包文件手工发布到 kando-plugins 的 GitHub Pages（例如推到它的 Pages 分支）。私钥仍只经环境变量，用完不留在 shell 历史里。

- P3c（已完成）：官方参考插件、官方插件的签名和打包、吊销列表的发布、Windows 上的插件测试。和正文的出入如下，4.2、6.4 已同步改过：
  - 位置：插件的源码在 `apps/kando/plugins/official/ankiconnect/`（`kando-plugin.json` 不写 `script`、入口 `plugin.tsx`、给 `nsgo publish` 读的 `nsgo.yaml`），和它的 Go 测试放在一起。放在 `apps/kando` 模块里是因为测试要用 `plugins`、`plugins/script` 在真沙箱里跑它；仓库根目录不是模块（`go.work`），放在根目录测试就要跨模块读文件。它不会进 app：没有任何包导入它，`.tsx` 也不被嵌入；NSGo 遍历 `apps/kando` 时遇到带 `nsgo.yaml` 的子目录会跳过（`ownsTree`）。非界面包 `plugins/official`（只导入 tsx/pkg 和 `plugins/script/signature`，不链接 TSX 运行时和界面）放打包和吊销列表的检查：`Wrap`（把 `nsgo publish` 签好的 `.nsgopkg` 和清单打成 `.kandoplugin`，清单的 `script` 按包的 SHA-256 填上，包的 App ID 和版本必须是清单写的，同一个包每次打出的字节相同）、`VerifyPackage`、`CheckRevocations`、`FileName`（`<id>-<version>.kandoplugin`）；`cmd/official` 是脚本和 Release 调用的命令行。官方根公钥只写一次，在 `signature.OfficialRoot`（#326 把验签挪进的非界面包 `plugins/script/signature`；`script.OfficialRoot` 是它的转出，`official`、`publish` 和 `cmd/kando-plugin` 都读它），`official` 和 `cmd/official` 也只经它取根和 `AppIDPrefix`，所以 Release 检查吊销列表时编译的只有 `official`、`signature`、`plugins` 和 tsx/pkg，不编译插件运行时。`cmd/official` 只放 owner 签名时用的 `wrap` 和 Release 用的 `revocations`；检查和列出目录里的包是 `cmd/kando-plugin` 的事（P5），两者不重叠。
  - 插件：贡献一个学习导出目标「Anki (AnkiConnect)」（`exportNotes`，收句子和生词），只申请一条网络规则（`POST http://127.0.0.1:8765/`）和可选的 `kando.sound.read`，不申请 `saved.read`、`words.read`（导出目标直接收到条目，P3b）、`settings.get`（设置随调用传入）、`lines.seen`、`llm.chat`，也没有命令。和 4.2 的示例相比少了 `run` 命令和事件，`optional` 只有声音。设置是牌组（默认 `Kando`）、笔记类型（`kando`、`basic`、`basic-reversed`，默认 `kando`）、附上声音（默认开）、标签（默认 `kando`，空格或逗号分开，留空也用 `kando`）。默认用 `Kando` 笔记类型，因为 Anki 按建库时的语言给自带的笔记类型起名，中文 Anki 里不一定有叫 Basic 的类型；`Kando` 没有时用 `createModel` 建（字段 Text、Meaning、Reading、Context、Sound、Source），已有但字段不对时报错，不改用户的类型。选 Basic 时按 `modelFieldNames` 取前两个字段当正面和背面，不管它们叫什么名字。
  - 流程：`requestPermission`（Anki 不认识调用方时会弹窗问用户；没有这个动作的旧版按「太旧」报）→ `version` 不低于 6 → `createDeck` → 笔记类型 → 每 100 条一批：先在本次导出内按正面文字去重（空的跳过），`canAddNotes` 判重（牌组内按第一个字段），只给要加的句子读声音（`kando.sound.read`）并 `storeMediaFile`（文件名 `kando-<收藏 id>.m4a`，同一句重发不会堆副本），再 `addNotes`。新版 AnkiConnect 只要有一条加不了就整批报错，这时改为逐条 `addNote`，加不了的算跳过；旧版对加不了的回 null，也算跳过。Basic 类型把声音放在背面，所以带不带声音不影响 Anki 的判重。字段内容按 HTML 转义，换行变成 `<br>`。
  - 时间：看懂的 runner 给一次调用一分钟（插件自己用的时间，宿主替它读声音的时间不算，6.6），超时就按 slow 停用插件。所以一次导出自己的时间限在 45 s 以内，每个请求最多 20 s（用 `AbortController` 中止），剩下不够 2 s 时不再发请求；超时一律以 `anki-timeout` 拒绝，原因里写明已经加进去几条，用户在 Anki 空闲时再导出一次，已加的会被 Anki 判重跳过。Anki 很忙时一次导出可能只送出一部分，这是 runner 的期限决定的上限，插件这一侧没法分成几次调用（导出是宿主的一次调用）。
  - 声音：插件给交给它的每一句都读声音。原来每分钟 60 次 host 调用的限额（6.2）会让一次导出只有前 60 句带声音，插件在一次调用里也等不起一分钟（runner 的期限），所以改在宿主：用户按下的导出进行期间，读声音按这次导出计，最多「句子数加 8」次，不计入每分钟的限额，调用结束额度作废（5.3，`pluginsAllowSounds`、`pluginsSoundAllowed`）。读声音被拒绝因此都是真的失败：对所有句子都成立的（没授权、没有 ffmpeg、仍然遇到 `quota`、读声音卡住被宿主的 120 s 超时放弃 `timeout`，或插件自己 150 s 的 `ETIMEDOUT`）让剩下的句子不再要声音；只和这一句有关的（`not-found`、`too-large`、`failed`、`busy`）只跳过这一句。导出的回答多了可选的 `soundless`（发出去但没附上声音的句子数，`plugins.Exported`），大于 0 时收藏句子面板说「<名称>：已发送 N 条，跳过 M 条，其中 K 句没有附上声音。」（`savedExportedPluginSoundless`），不再只说发了几条。
  - 错误：用户能处理的失败都在 async 函数里以带 `code` 的 Error 拒绝（清单写 `>=0.36.0`，而 v0.37.0 及更早的看懂里同步 throw 会丢掉 code，nsgo#847），熔断不计：`anki-unreachable`（Anki 没开或没装 AnkiConnect）、`no-network`（用户没授权联网，请求在发出前就被拒绝）、`anki-denied`、`anki-api-key`（AnkiConnect 设了 apiKey，本插件不发 key）、`anki-version`、`anki-note-type`、`anki-bad-reply`（端口上不是 AnkiConnect）、`anki-error`（AnkiConnect 自己报的错）。原来收藏句子面板对插件导出的失败只说「<名称> 没能接收这些句子」，看不出原因；现在插件带 code 拒绝时，面板说「<名称> 没能接收这些句子：<原因>」（`savedExportPluginFailedReason`），原因是插件自己的文字（通常是英文），经 `plugins.Reason` 去掉换行、控制符和 Bidi 字符，最多 200 字；只在 code 是插件自己的时显示（`pluginsOwnCode`）：没有 code、看懂拒绝 host call 用的 code（`denied`、`quota`、`timeout` 等，插件原样往外抛时）、运行时的 `ETIMEDOUT` 和 `ERR_…` 都不显示原文。插件本来就能用 `kando.toast` 在自己名下显示任意文字，所以这不扩大 T4 的风险。
  - 清单和代码一致：`plugin.tsx` 直接 `import manifest from "./kando-plugin.json"`，`kandoManifest()` 返回其中签名覆盖的那几个键，清单被编译进签名的 bundle，两者不会走样（4.3）。`nsgo.yaml` 的 id、版本和能力由测试 `TestPluginAnkiConnectPublishes` 对着清单检查，能力必须恰好是清单的 `capabilities` 加 `optional`。
  - 测试（`TestPluginAnkiConnect*`，CI 的三个系统和 Race 都跑）：用测试现场生成的密钥按开发者打包的方式签这个文件夹（`script.PackFolder` 给发布者 `kando` 签证书，测试把这把钥当官方根），再用 `official.Wrap` 打成 `.kandoplugin`、`plugins.Stage` 安装、`script.Verify` 验出来源是「官方」、`script.Probe` 比对清单，最后在看懂的沙箱（`script.Load`、`script.Runner`）里调 `exportNotes`。插件的 `fetch` 经 `tsx.WithFetcher` 换成测试的网络：网络规则的检查照常在发出前做，过了检查的请求交给一个 httptest 服务器上的假 AnkiConnect（不管请求写的是哪个主机和端口），所以不占 8765 端口，也不用改签名的规则；P3b 的 `credentialFetch` 测试也是这样换掉拨号的。覆盖：建笔记类型和牌组、句子带声音、生词、HTML 转义、标签（留空时用 `kando`）、本次导出内的重复和空文字、再导出全部跳过且不再读声音、Basic 的字段映射和缺少 Basic 时报错、每批最多 100 条、整批失败后逐条添加、Anki 没开和五种 AnkiConnect 的拒绝各自的 code、没授权联网时一个请求都不发、没授权声音时不读声音且计入 `soundless`、200 句全部带上声音（插件一侧）、Anki 每个请求要 12 s 时在 runner 的期限之内以 `anki-timeout` 结束且插件没被停用（约 45 s，`-short` 时跳过）、关掉声音设置；runner 用的是看懂的 `DefaultWatch`（只放宽心跳），测试结束时断言 runner 没有停用过插件；每个用例都断言所有请求都是 `POST http://127.0.0.1:8765/`。`plugins/official` 的测试覆盖 `Wrap` 和吊销列表的检查，app 层 `TestPluginContribExporter` 加了带 code 失败时面板显示原因、转抛看懂的 `quota` 时不显示、`soundless` 的那句话，`TestPluginExportReasonIsThePluginsOwn` 逐个检查哪些 code 显示原因；`TestPluginExportReadsEverySound` 在真宿主上导出 200 句、每句都读到声音（没有这项改动时只有 60 句），调用结束后额度不再存在，之后插件自己的读声音第 61 次仍被以 `quota` 拒绝。
  - Windows：CI 的 Test（Windows）一行本来就跑全部模块，插件测试也在里面，但结果可能来自测试缓存；新增的「Plugin runtime」一步在 Windows 上用 `-count=1` 再跑一遍 `TestPlugin|TestScript`（`./apps/kando/` 和 `./apps/kando/plugins/...`），保证每次都是在 Windows 上真的跑过。
  - 官方插件的签名和打包：`.github/scripts/official-plugins.sh`（`make official-plugins`）在 owner 自己的机器上运行，不进 CI。它对 `apps/kando/plugins/official/` 下每个带 `nsgo.yaml` 的文件夹（或参数给的文件夹）跑 `go tool nsgo publish`，用 `NSGO_RELEASE_KEY` 里的官方发布密钥和 `KANDO_OFFICIAL_CERT` 指向的证书（`kando-official.cert`，放在仓库外，放在仓库里会直接拒绝），再用 `cmd/official wrap` 检查包链到官方根、是清单说的那个插件，然后按 kando-plugins 收脚本插件的布局写出：`scripts/<id>/<id>-<version>.kandoplugin`，以及签它用的源码 `scripts/<id>/src/`（插件文件夹里除 Go 测试以外的文件，kando-plugins 的 CI 检查 `src/kando-plugin.json` 和包里的清单除 `script` 外一致）。写到 `KANDO_PLUGINS_OUT` 指的文件夹，没设时新建一个临时文件夹，脚本最后打印它的位置；`KANDO_PLUGINS_OUT` 指向 kando-plugins 的本地克隆时直接写进去，指向本仓库里会直接拒绝（包不再从本仓库发布）。已有同名文件时拒绝覆盖：缓存了旧索引的看懂按 SHA-256 校验文件，新版本必须换文件名；同一个 `scripts/<id>/` 里还有旧版本的文件时，脚本提示在同一个 PR 里删掉它（kando-plugins 每个文件夹只列一个版本，它的 Publish 会把下架的旧文件再保留一周）。之后在 kando-plugins 开 PR 提交 `scripts/<id>/`，照上面「发布社区索引」的步骤审核、合并、签索引。原先（P3c 合并时）脚本写到 `apps/site/public/plugin/packs`，由 owner 在本仓库提交，P5 目录发布之后改成了现在的做法，那个目录已经不存在。
  - 吊销列表的发布：`.github/scripts/plugin-revocations.sh`（`make plugin-revocations REVOKE='-keys <key ID>'`）在保管根密钥的机器上运行，用 `NSGO_ROOT_KEY` 跑 `go tool nsgo revoke`：序号是已提交列表的加一（第一次是 1），吊销内容是已提交列表的全部再加上这次给的（看懂对每个根只保留最新的一份列表，新列表不带上旧的条目就等于撤销吊销），默认 180 天过期（`-days`），写到 `apps/kando/plugins/revocations/kando-plugin-revocations.json`，写之前先按 Release 的规则检查一遍；owner 提交这个文件。Release 工作流的 Publish 作业在附上 browser extension 之后：没有提交列表时只记一条 notice、不附（看懂把 404 当作没有列表）；有列表时用 `cmd/official revocations` 检查它由官方根签名、没有过期、不比最新已发布版本附的那份旧（序号更小，或序号相同但内容不同，都算旧；序号相同、内容相同是正常情况，大多数发布都会原样再附一次），不通过就让整次发布失败，通过后作为 `kando-plugin-revocations.json` 附上并写进 `checksums.txt`（随包一起做来源证明）。剩不到 30 天过期时检查打出 warning。根密钥从不进 CI。
  - 代价：一旦发布过列表，owner 就要在它过期前续签（不吊销新东西也要续，`make plugin-revocations`），否则 Release 会失败；而已经拿到这份列表的看懂在它过期后会暂停申请了网络或凭据的非官方插件（6.4）。还没发布列表时这些都不发生。
  - 本 PR 没有生成任何真实的包或列表，要由 owner 用上面的命令签好后发布。之后 owner 已用官方发布密钥签好 `kando.ankiconnect` 1.0.0 和 `kando.deepl` 1.0.0，连同 `src/` 放进了 kando-plugins 的 `scripts/kando.ankiconnect/`、`scripts/kando.deepl/`，两者都以来源 `official` 列在 `https://kando.yldm.ai/plugin/index.json` 里；吊销列表 `kando-plugin-revocations.json` 还没有发布过。签名流程用测试现场生成的根、发布密钥和证书在 Ubuntu 编译机上手工跑通过一次（`nsgo certify`、`publish`、`cmd/official wrap`、`Stage`、`Verify`、`Probe`，以及 `revoke` 和三种检查结果），没有提交这些产物。
  - 没有提交 NSGo issue：这一期用到的 `tsx.WithFetcher`、`pkg.Inspect`、`pkg.NewTrust`、`UpdateRevocations` 都按文档工作。

**签名和发布官方插件、吊销列表**（第 4 条的根密钥命令之外，owner 用到的全部命令）：

```sh
# Once a year, on the offline machine: the plugin root certifies the official release key (kept outside the repository).
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'ai.yldm.kando.plugin.kando.*' -days 365 -out ~/keys/kando-official.cert
# Sign every official plugin (apps/kando/plugins/official/*) into a kando-plugins checkout, as scripts/<id>/<id>-<version>.kandoplugin with its source in scripts/<id>/src/; PLUGINS=<folder> signs one, and without KANDO_PLUGINS_OUT it writes into a new temporary folder and prints where.
NSGO_RELEASE_KEY=<release secret> KANDO_OFFICIAL_CERT=~/keys/kando-official.cert KANDO_PLUGINS_OUT=~/src/kando-plugins make official-plugins
# Check a file the way Kando installs it: it must report source official.
kando plugin validate ~/src/kando-plugins/scripts/kando.ankiconnect/kando.ankiconnect-1.0.0.kandoplugin
# Then, in the kando-plugins checkout, commit scripts/<id>/ on a branch and open a pull request; its Publish signs the index on merge.
# Revoke a release key or a package, or renew the list before it expires (no REVOKE); then commit apps/kando/plugins/revocations/kando-plugin-revocations.json.
NSGO_ROOT_KEY=<root secret> make plugin-revocations REVOKE='-keys <key ID> -packages <sha256 or .nsgopkg>'
NSGO_ROOT_KEY=<root secret> make plugin-revocations
```

- 发新版本的插件：改代码后把 `kando-plugin.json` 和 `nsgo.yaml` 的 `version` 一起改大（`nsgo publish` 按版本推算 build，build 必须变大，否则看懂按防回滚拒收），再跑 `make official-plugins`，在 kando-plugins 的 PR 里删掉 `scripts/<id>/` 里的旧版本文件、加上新的（kando-plugins 的 Publish 把下架的旧文件再保留一周，缓存了旧索引的看懂照样下得到）。
- 吊销列表提交后，下一次发布才会附上；需要马上生效时手动触发一次 Release（`workflow_dispatch`）。

- nsgo#761 跟进（`refactor/plugins-signed-contents`）：上游给包加了 app 自己签的元数据（`pkg.Contents.Extra`、`Info.Extra`）。和正文的出入如下，4.3、12.1 已同步改过：
  - 消费端：`signature.Verify` 在包签了清单时直接比对（`signature.CheckManifest`，比对规则从 `script` 挪进 `signature`，`kando plugin validate` 和插件目录的 CI 工具因此也会比对签了清单的包），`script.Probe` 对这种包不再跑代码，导出函数的检查同时加进 `script.Load`（对两种包都查，没签清单的包在 `Probe` 里已经查过一次）。没签清单的包照旧走 `kandoManifest()`，原来的 `TODO(nsgo#761)` 删掉了：这条路要一直留着给已经发布的包用，不是绕行。
  - 签发端：`nsgo publish` 签不了 `Contents.Extra`，所以 `official-plugins.sh` 和 `script.PackFolder` 都还不签清单，留了 `TODO(nsgo#924)`（[nsgo-dev/nsgo#924](https://github.com/nsgo-dev/nsgo/issues/924)）；`official.Wrap` 已经会检查签了清单的包（`ErrSignedManifest`、`ErrMismatch`）。签了清单的包在看懂 0.37.0 及以前会被拒装，所以只有清单写 `"kando": ">=0.38.0"` 或更高的插件才签（`signature.SignsManifest`）。0.38.0 只保证不拒装：比对签名清单的第一个版本要等本改动发布后才知道，在清单下限达到它之前，签了清单的插件仍要导出 `kandoManifest()`（4.3）。
  - 测试：`TestScriptSignedManifest`（签了清单的包：顶层代码会抛错、也没有 `kandoManifest` 时照样通过验签和预读；缺导出时 `Load` 拒绝；清单比签的多、签了别的键、签的不是对象都按 `ErrTampered` 拒绝；签好后改一个字节按 `ErrUntrusted` 拒绝），`TestPluginOfficialWrapSignedManifest`（`Wrap` 收下下限够新且一致的包，拒绝不一致的和下限太旧、没写下限的）；没签清单的一路仍由 `TestScriptProbe` 等原有测试覆盖。线上两个 1.0.0 文件用新代码验签、预读通过，只做了一次，没有提交成测试（文件不在仓库里）。
- P5 E15（已完成）：播放器侧栏的「扩展」标签。和正文的出入如下，2.1、2.3、5.3、8.4、第 9 节已同步改过：
  - 代码位置：非界面包 `plugins` 加了 `Panel`、`PanelIcons`（`contrib.go`）和 `EventLineChanged`（`perm.go`）；`plugins/script/panel.go` 是 `Runner.Panel`（唯一的 `tsx.View`）和 `Runner.Failed`；界面是 `film/extensions.go`（`film.Extender`、`SideExtensions`、标签正文），`film/panel.go` 的 `panelTabs` 改成 `screen.panelTabs`；app 一侧是 `state_plugins_panels.go`。共享文件只动了插件块自己的：`pluginsScript` 加 `panel`、`run` 两个字段，`pluginsGuard` 也认面板视图的 Host，`pluginsSend` 同时发给面板视图，`savedPlugins` 加 `PluginPanel`，`pluginsHooks` 的 `playing`、`afterChange` 各加一行 `pluginsPanelsLine`；`settings.PluginAdds` 加一行面板。
  - 「`var panelTabs` 改成 `film.App` 上的方法」：`film.App` 是接口，标签的 id 是 film 自己的知识，所以改成 film 的 `screen.panelTabs`（`screen` 持有 `film.App`），由新的 `film.Extender.Extensions` 决定「扩展」出不出现；没有面板时 `FilmSide` 仍记着 `extensions`，侧栏照字幕显示，插件回来时直接回到「扩展」。加了「扩展」后七个等宽的标签在 340 pt 的侧栏里放不下英文、日文的标签，所以「扩展」画成只有图标的标签（Lucide `puzzle`，读屏名称和悬停提示都是 catalog 里的「扩展」，切换语言时重建这一个标签），其余标签按文字宽度（左右至少 3 pt，再一起铺满整条），七个标签在中、英、日下都放在一行里；万一哪种语言放不下，整条横向滚动，并把选中的标签滚进视野。`TestTabLabelsFit` 在中、英、日下检查每个标签都整个落在条里、条不需要滚动、每个文字完整、「扩展」的读屏名称；截图场景在 `KANDO_SCREENSHOTS_LANG` 的语言下做同样的检查。
  - 面板除了 `{id, title, icon?}` 还要写 `export`：和其余四种贡献一样点名插件导出的函数，`Probe` 据此检查它确实导出了。NSGo 缺口：升级到 f377daba0a2a 之前，宿主没法把包的某个导出当作视图渲染（下面的行号是升级前的版本）。`tsx.View` 只显示入口模块自己 `mount` 的东西（`tsx/view.go:26-31` 经 `Sandboxed` 跑整个包，`tsx/sandbox.go:135-142` 取的是运行时的根），`tsx.Plugin` 只有 `Call`，返回值经 `exportJSON` 变成 JSON，画不出节点（`tsx/view.go:118-177`）。所以入口模块要读 `host.params`（`tsx/host.go:20-22`，`core.Host.SetParams`）自己 `mount`，提交为 [nsgo-dev/nsgo#883](https://github.com/nsgo-dev/nsgo/issues/883)。上游已加 `tsx.View.Export`（`tsx/view.go`，`tsx/sandbox.go:200` 的 `renderExport`），但这还不够 Kando 用：`renderExport` 只把入口模块挂上的节点从根上摘掉（`tsx/sandbox.go:222-224`，`removeChild` 的注释写着节点留到它的 owner 结束，`tsx/dom.go:241`），没有销毁 `mount` 建的 owner（`tsx/dom.go:728`）。照 0.37 的约定自己 `mount` 的插件，如果宿主再按 `Export` 渲染一份，挂上的那份会一直跑着：effect、定时器和 `host.on` 都执行两遍（事件处理两次、`host.call` 调两次、看门狗看到双倍负担）；根是函数、`If` 或数组时，它重新渲染会把节点再插回根上，面板里出现两份（实测：根从一个节点变成两个后显示了四个）。Kando 看不出入口模块挂没挂，没法只对新写法的插件用 `Export`，所以 `Runner.Panel` 仍只显示入口模块挂上的东西，插件仍要自己 `mount`；代码里的 `TODO(nsgo#923)`（[nsgo-dev/nsgo#923](https://github.com/nsgo-dev/nsgo/issues/923)） 记着这个缺口，等 `renderExport` 销毁入口模块挂上的东西后再直接渲染 `Panel.Export`，并照旧设置参数，让老插件不用改。
  - 一个 `core.Host` 同一时刻只服务一个 app（`core/host.go:239-253` 的 `serve` 把 `h.app` 换成新的），所以面板视图不能和插件函数共用 Host，否则插件就收不到事件了。面板视图的每次构建（包括「重试」）都新建一个 Host，handler 相同；旧 Host 的调用被 `pluginsGuard` 拒绝，旧 Host 里排队的事件（最多 `core.HostInbox` 条）也不会在重试时一起重放、把配额用光。视图出错时 `pluginsScript.panel` 清空，事件不再发给它。
  - 看门狗：面板视图不在 `Runner` 的调用里，调用的心跳、1 分钟期限和堆阈值管不到它，所以它有自己的看门狗（`script.PanelWatch`，`DefaultPanelWatch`）。NSGo 缺口：tsx 不报告沙箱运行时的 JS 跑了多久，也没有包住它的定时器、帧和事件的钩子（`tsx/timers.go:54-72` 经 `Scope.AfterFunc` 和 `runtime.call` 跑定时器，没有回调；`tsx/sandbox.go:458-504` 给每次调用计时只用来切断）。[nsgo-dev/nsgo#836](https://github.com/nsgo-dev/nsgo/issues/836) 关掉时只加了 `Limits.CallTotal` 和 `Plugin.Interrupt`，两样都只管 `tsx.Load` 载入的插件（`tsx/view.go:83`），放 `tsx.View` 的一方既拿不到视图的运行时间，也没有办法打断它，所以面板这部分还是缺口，代码里留了 `TODO(nsgo#925)`（[nsgo-dev/nsgo#925](https://github.com/nsgo-dev/nsgo/issues/925)）。所以看门狗只能看整个引擎：每个窗口里均匀地投 50 次心跳，等待超过 4 ms 的心跳把它到下一次心跳之间的时间算作忙；连续 3 个 5 s 窗口忙于 60% 以上，或活堆（`runtime.GC` 之后）比视图建好时多出 `Watch.Heap`（512 MiB），就怀疑这个视图。为了不误伤，怀疑之后先把视图撤下（`core.Show` 收起，它的运行时、定时器随之销毁），从撤下生效时起测 2 s：引擎忙不到 30%，或两次 GC 后活堆回到阈值以内，才按 `slow`、`memory` 停用插件；否则负担是 app 自己的或别的插件的，重建视图、从头计数，连续 3 次冤枉后这个视图在重建前不再被怀疑。残余：被冤枉时视图会重建一次，面板的内存状态丢失；一个把工作切成很短的片、让引擎忙不到 60% 的面板不会被停。
  - 熔断：面板视图的每次失败都经 `Runner.Failed` 计数（规则和调用一样，带 `code` 的 `core.Error` 不算），没出错就被撤下（换了面板、关了侧栏）的每次构建经 `Runner.Succeeded` 记一次成功，所以相隔很远的失败不会累积成停用；最近 50 次里 3 次失败就按 `failed` 停用插件，health.json 和 audit.log 照常记。计数都在建树之后经 `Runner.Post` 做，因为停用会改 app 的状态。「重试」重建视图，再失败也算一次。
  - in-flight 标记只盖每次构建入口模块同步运行的那一段，由 `Runner` 唯一的写标记 goroutine 按顺序写和清（`Runner.mark`），建树时不读写磁盘；代价是 Kando 恰好在入口模块里崩溃而标记还没写下时，下次启动不会停用它。这个插件正有调用在跑时不写，否则会清掉调用自己的标记。
  - 退出时等插件文件夹的写入：插件块在后台读写插件文件夹的工作（state.json、health.json、audit.log、插件设置的值、trust 的 build 记录、吊销列表、扫描和安装）都经 `pluginsWrite` 计数，`quitting` 钩子先停掉面板的标记写入（`script.Runner.Drain`，之后一个标记也不写），再等这些工作结束（最多 5 s），免得进程结束时把文件写了一半；测试结束时 `newTestApp` 同样等它们，临时目录才删得掉。这修的是 main 上已有的偶发失败：macOS 上 `TestPluginWaitingIsCapped`、`TestPluginContribSubtitleFormat` 的「TempDir RemoveAll cleanup: directory not empty」在 main（7fbdbaf7）上 `-count=20` 也有 8 次，起因是停用插件后 health.json、state.json、audit.log 的写入在测试结束后才落盘，和面板无关。
  - `line.changed` 原来不推送（5.3）。现在只推给正在显示的面板视图：数据是行号，正文要插件有 `kando.line.current` 的授权自己取，没授权时不推。为函数载入的插件仍收不到它，免得每次换句都在 Runner 外跑插件的 JS。同一轮里（以及下一轮之前）的多次换句合成一个事件，带的是发出时的行（`pluginsPanelsSend`）；换了影片或新建了面板视图，下一次都会再推一次当前行。插件建视图时自己读一次 `kando.line.current` 更稳。
  - 正文只在换了面板或插件重新载入时重建；但侧栏本身收起再打开（`ns.Presence`）、换到别的标签再回来都会重建面板视图，入口模块会再跑一次，面板自己的内存状态要放进 Storage 才能留住。
  - E5 说 task 包的结果也可以放进这个标签：没有做。task 的结果已经有自己的侧栏视图（`SideTask`，⋯ 菜单和命令面板进入，带返回按钮），搬进「扩展」要么让 task 包也出现在面板选择器里（但它没有视图、只在运行后才有内容），要么在标签里混两种来源，选择器的语义会变复杂；保持原样。
  - 下拉框的选项绑定在 `ui.Select` 的 `OptionsFunc` 上（nsgo#848），面板增减时原地更新；`Label` 只读一次，所以下拉框放在一个只读界面语言的 `core.Dynamic` 里，切换语言时重建（`TODO(nsgo#145)`）。
  - 2.3 原说插件设置页以声明式 schema 为主、`tsx.View` 作退路：没有做退路。清单没有设置页视图的字段，详情页只画 7.3 的声明式设置，`tsx.View` 只用在这个标签的面板上；2.3 已改成这样。
  - 测试：`plugins` 的贡献校验（`contrib_test.go`，含图标不在列表里、图片当图标）；`plugins/script` 的 `TestScriptPanel`（按参数挂对应的导出、函数载入不挂、抛错显示 Fallback、每次构建一个新 Host、三次失败停用）、`TestScriptPanelShowsWhatItsModuleMounts`（入口模块挂上的根是函数、重新渲染成两个节点后仍只出现一份；换成 `tsx.View.Export` 渲染时这个测试会失败，见上面的 NSGo 缺口）、`TestScriptPanelPastItsBudget`、`TestScriptPanelWatchdogStopsABusyPanel`（setInterval 让引擎忙的面板按 `slow` 停）、`TestScriptPanelWatchdogStopsAGrowingPanel`（一直留着字符串的面板按 `memory` 停）和 `TestScriptPanelWatchdogSparesAQuietPanel`（app 自己忙时安静的面板不被停）；不变量测试改成 `tsx.View` 只能在 `plugins/script` 的非测试代码里、`Sandbox` 字段是 `Sandbox(...)` 的字面量、并且写成 `tsx.View{…}.Render` 当场渲染（不能先放进变量再改字段），并要求至少有一处；`film` 的 `TestThePanelShowsTheExtensions`、`TestTabLabelsFit` 和 `TestEveryPanelIconIsDrawn`；app 的 `TestPluginPanels`（打开播放器、出现「扩展」、面板经授权读当前行、播放换句时收到 `line.changed` 且不重建、一轮里的多次换句合成一个事件、换到会抛错的面板显示出错而播放器还在、出错时没有 Host 收事件、重试也算失败、三次失败停用插件并记 health、选择存进 settings.json、标签消失）和 `TestPluginPanelsGoWithThePlugin`。截图场景 `plugins-script-panel` 在 Linux 上按中、英、日跑了无头断言（含标签宽度），协调方在 macOS 上出了中文的图。
- 授权页补齐（已完成）：6.2、6.6、6.7、7.1、7.2、7.3 已同步改过。和正文、和审计时的提议的出入如下：
  - 6.2 原来写「勾一项时，它依赖的能力一起勾上（例如 `llm.chat` 依赖 `lines.seen`）」。实际上没有任何 handler 依赖另一项权限，`llm.chat` 也不读字幕，所以没有做自动勾选，改成如实写「没有依赖」；将来真有依赖时再在 `plugins/perm.go` 加。
  - 「规则限制不了」只看网络规则（路径是 `/` 或前缀是 `/`），不看凭据的 pathPrefixes。kando-plugins 仓库 CONTRIBUTING 里的同一句不在本仓库，没有改。
  - 6.7 的五条里上授权页的是三条（见 6.7 最后一条）。凭据库那一条按 `runtime.GOOS` 判断（`pluginsKeysShared`，macOS 以外都写），所以在 macOS 上出的截图里没有这一行。
  - 「需要授权」「离线时不可用」放在 `PluginScript.Problem` 里，和其余停用原因共用行内和详情顶上的位置，排在它们后面。「需要授权」对刚装好的插件也显示，因为这时下一步正是授权；它不灰掉开关，插件照样能按已授予的部分运行。
  - 详情原来的标题「权限」（`settingsPluginsPermissions`）换成了四组的标题，旧键从五份 catalog 里删掉了。
  - 脚本插件的安装记录原来在 `plugins.Swap` 之前写，现在和数据包一起在包放到位之后写，装失败的不再记成安装。
  - 截图场景：`plugins-script-install`、`plugins-script-install-official`（大小、规则限制不了、残余风险）、`plugins-script-details`（四组标题、需要授权、规则限制不了、残余风险）、`plugins-script-credential`（「联网」组标题、残余风险）变了，新增 `plugins-script-rows`（一行需要授权、一行离线时不可用）。
  - 测试：`plugins` 的 `TestScriptGrantStates`（分组、`Missing`、`Online`、`Unscoped`、`Size`）；根目录的 `TestPluginScriptGrantPage`（确认框的大小和两行、详情的分组和顺序、行内的两种状态、取消、安装和授权的审计记录）、`TestPluginScriptKeysSharedWithOtherPrograms`、`TestPluginsAuditLog`（取消、site 包安装带域名、逐域名同意和撤回、卸载），`TestPluginTaskRunsOnTheFilmSoFar` 加了 task 包开关和 token 的审计记录。

### P3.0 spike 的测量结果

2026-11-19 测量。测量代码只是一次性的，量完已删，没有提交。NSGo 固定在 `v0.1.1-0.20261118094851-ec272782be28`，Go 1.27.4。linux/amd64 和 windows/amd64 在 Ubuntu 编译机（x86_64，24 核）上编译，运行时只在 Ubuntu 上跑；darwin/arm64 在 Mac Studio（arm64，cgo，外接 PCIe SSD）上编译和跑。Windows 只量了体积。测量当天上游 #759、#760、#761 都还开着、没有评论，main 在固定版本之后没有改 `tsx/sandbox.go`、`tsx/view.go` 和 `tsx/pkg` 的相关逻辑。

**1. 二进制体积**。命令是 `go build -trimpath -tags=nodra_nomygo -ldflags='-s -w' ./apps/kando`（CI 和 `nsgo.yaml` 用的 tag），对比加不加一个带 build tag 的文件。这个文件在 `init` 里按环境变量调用 `pkg.OpenTrust`、`AddRoot`、`Verify`、`Accept`、`tsx.Load`、`Call`、`Exports`、`Close`，让链接器保留 P3 会用到的路径。

| 目标 | 不链接 tsx | 链接 tsx + tsx/pkg | 增量 |
|---|---|---|---|
| linux/amd64 | 56.31 MB | 77.61 MB | +21.30 MB（+20.3 MiB，+37.8%） |
| windows/amd64 | 57.45 MB | 79.17 MB | +21.72 MB（+20.7 MiB，+37.8%） |
| darwin/arm64 | 55.23 MB | 75.15 MB | +19.91 MB（+19.0 MiB，+36.1%） |

不 strip 时增量是 24–26 MB，压缩后的安装包没有量。按符号大小分包看，goja 本身约 1.8 MB，tsx 约 1.2 MB，regexp2 约 0.4 MB，tsx 注册的 NSGo `base`、`ui` 组件和 `base/plot` 约 1–2 MB；其余大部分来自链接器的方法裁剪失效。goja（`object_goreflect.go:222`）和 tsx（`goface.go:1248-1249`、`classes.go:63`、`messages.go:44`）都用非常量参数调 `reflect.Value.Method`、`reflect.Type.Method` 或 `MethodByName`，链接器因此保留所有可达类型的全部导出方法。例如 tsx 完全没用到的 `aws-sdk-go-v2/service/s3` 也大了 2.3 MB，`apps/kando/locales` 大了 0.4 MB。这一部分 Kando 这边削不掉。`go mod tidy` 之后 `apps/kando/go.mod` 多两行 indirect，见 3.2 节。

**2. 引擎 goroutine 的阻塞**。用 `core.NewRoot` 加一个独占 goroutine 的 post 队列模拟引擎，按 3.3 节的 Limits（`DefaultLimits` 为底，Load 1s、Call 500ms、Timers 64、Requests 4、Body 4 MiB、Storage 1 MiB）载入，Grant 只给 `host`。另一个 goroutine 每毫秒往同一个队列投一个空函数，记录它等了多久，也就是一个输入事件要等多久。nsgotest 下的数字相同（Load p50 0.87 ms，同步 Call p50 1.4 µs）。

| 操作 | Ubuntu x86_64，p50 / p99 | Mac Studio arm64，p50 / p99 |
|---|---|---|
| Load 空包（1.9 KB 源码） | 0.83 / 2.1 ms | 0.60 / 3.5 ms |
| Load 200 个导出函数（115 KB） | 4.9 / 6.8 ms | 3.1 / 6.5 ms |
| Load 2000 个导出函数（1.15 MB） | 46.8 / 53.2 ms | 29.9 / 47.2 ms |
| Call 同步的 `add(1, 2)`，引擎上占用的时间 | 1.3 / 10 µs | 1.1 / 17 µs |
| Call `async ping()`，从投递到 done | 12 / 24 µs | 10 / 39 µs |
| 一次 Call 里 1 万次 `await host.call`，handler 同步 Resolve | 共 111 ms，每次往返 11 µs；期间输入事件等待 p50 74 µs、p99 0.97 ms | 共 60 ms，每次 6 µs；p50 31 µs、p99 1.5 ms |
| in-flight 标记：写临时文件、rename、删除，不 fsync | 46 / 125 µs（ext4） | 150 / 392 µs（APFS） |
| 同上，加 fsync | 0.81 / 1.6 ms，最大 8.6 ms | 3.3 / 11 ms，最大 70 ms |

正常插件对引擎的占用是毫秒级，Load 每个 tick 只载一个（5.2）足够。in-flight 标记不 fsync（5.2）。

**3. 两个已知的洞，以及 Kando 能自己做的缓解**

ReDoS：`new RegExp("(a+)+\\1$").test("a".repeat(n) + "!")` 在引擎上跑，n=20 卡 0.42 s，22 卡 1.07 s，24 卡 4.2 s，28 卡 71 s，32 超过 300 s 还在跑（每多一个字符时间翻倍，推算约 19 分钟）。500ms 的预算要等匹配结束才报 ErrBudget，这段时间里输入事件一直在排队。`^(a+)+$` 一样：这个版本的 goja 所有正则都走 regexp2，没有 RE2 的快路径（`goja/builtin_regexp.go:184-259`）。

大分配：同步的死循环，500ms 时被 ErrBudget 切断。基线 RSS 37 MB。

| 写法 | 峰值 RSS | Go 堆峰值 | Close 加 GC 之后 |
|---|---|---|---|
| 每次 push 一个 `new Array(1000)` | 156 MB | 123 MiB | 36 MB |
| 拼字符串 | 460 MB | 411 MiB | 44 MB |
| 小对象 | 359 MB | 307 MiB | 44 MB |

`debug.SetMemoryLimit(1 GiB)` 不改变结果，因为数据都被插件引用着，GC 收不走。

比 #759、#760 更要紧的两个新发现（3.3 节已写进运行时语义）：

- **ErrBudget 不停掉插件**：一个把数据存进模块级数组的导出函数，每次调用在 500ms 内多占约 250 MB，每次都被切断，但连调 8 次后 RSS 到 1.74 GB，直到 Close 才释放。
- **一次调用可以永远不结束**：`for (;;) { 忙 450 ms; await new Promise(r => setTimeout(r, 0)) }` 的每个片段都不超过 500ms，从不被切断，5 s 里输入事件等待 p50 449 ms，窗口等于卡死；同样的循环每个片段分配 20 万个对象，5 s 涨到 2.0 GB，一直不停。

试过的缓解（都不 fork NSGo）：

| 手段 | 结果 |
|---|---|
| main 在任何 Load 之前设 `regexp2.DefaultMatchTimeout = 100 * time.Millisecond` | 有效。goja 用 `regexp2.Compile` 编译每个 JS 正则（`goja/regexp.go:58`），会带上这个默认超时；超时后 goja 抛 `RangeError: match timeout after 100ms`（`goja/regexp.go:138-139`）。n=32 的匹配 0.2 s 就返回（regexp2 的时钟粒度约 100ms）；插件 catch 住再循环匹配，0.6 s 时被预算切断。代价：这是进程级的全局变量，要在启动时、任何正则编译之前设好；Kando 里目前只有 goja 用 regexp2；`github.com/dlclark/regexp2/v2` 要成为 `apps/kando/go.mod` 的直接依赖。 |
| Load 前静态扫描 bundle：用 goja 的 parser 遍历 AST，拒收含嵌套量词、反向引用或环视的正则字面量，以及 `RegExp` 标识符和 `.match`、`.search`、`.matchAll` 调用 | 只能当 lint。字面量、`new RegExp("…")`、`s.match("…")` 都扫得出来；用 `(/x/).constructor` 拿到构造函数、用计算出来的属性名调 `match`，都扫不出来，而且照样卡 1 s 以上。有了上一条就不需要它。 |
| 每个插件一个独立的图：`core.NewRoot(core.RootOptions{Post: …})` 加 `core.NewTree(root, nil)`，在专门的 goroutine 上 Load 和 Call | 可行。Load、Call、`host.call`、setTimeout、Close 都正常，`-race` 干净；handler 在插件的 goroutine 上跑，要读 app 的状态就转到引擎上，再从任意 goroutine Resolve。n=28 的正则卡住插件的 goroutine 时，引擎上输入事件等待 p99 27 µs。缺点：不调 `NewTree` 时 Load 返回空指针错误；这个图的 Storage 只能在内存里，重启就丢（见下面 NSGo 缺口 1）；卡住的 goroutine 回收不了，要等进程退出；内存仍然是同一个进程的。不作为默认做法。 |
| 看门狗：全局串行的 Call，一个 goroutine 定时往引擎投心跳，用 `runtime/metrics` 读 `/memory/classes/heap/objects:bytes`，超阈值就在引擎上 `Plugin.Close()` | 有效，但只能在片段之间生效：tsx 不暴露 Interrupt，看门狗自己打断不了 JS。分片分配的插件 Close 等了 47 ms，CPU 分片的插件等了 0.4 s（等当前片段跑完），Close 之后 GC 回到 84–90 MB。单个同步片段的上限由 500ms 预算决定，约 0.3–0.5 GB。写进 6.6 节。 |
| `debug.SetMemoryLimit` | 无效，见上。 |

**4. 根密钥、证书、签包和吊销的命令**（固定版本的 `cmd/nsgo/main.go:169-220` 和 `cmd/nsgo/publish.go`）。私钥只从环境变量 `NSGO_ROOT_KEY`、`NSGO_RELEASE_KEY` 读，工具不写私钥文件，也不接受私钥 flag。

```sh
# On the offline machine: once for the root, once per release key. Prints "public: <base64>" and "secret: <base64>", writes nothing.
go tool nsgo keygen
# The root certifies a release key for the official publisher segment.
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'ai.yldm.kando.plugin.kando.*' -days 365 -out kando-official.cert
# Compile the plugin folder (nsgo.yaml + entry) and sign it.
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert kando-official.cert -entry plugin.tsx -out dist/<id>-<version>.nsgopkg <plugin dir>
# Check it the way Kando will.
go tool nsgo verify -root <root public> -revocations revocations-3.json dist/<id>-<version>.nsgopkg
# Revoke release keys (by key ID) and packages (by SHA-256 or file); the sequence rises per root.
NSGO_ROOT_KEY=<root secret> go tool nsgo revoke -sequence 3 -days 30 -keys <key ID,...> -packages <sha256 or .nsgopkg,...> -out revocations-3.json
```

- `keygen` 打印的公钥是 32 字节公钥的标准 base64，私钥是 32 字节 seed 的标准 base64；key ID 是公钥 SHA-256 的前 8 字节的 hex（`pkg.KeyID`），`keygen` 和 `verify` 的输出里都有。
- `certify` 的 `-apps` 是逗号分隔的 `*`、`前缀.*` 或完整 id，`-days` 默认 365，`-out` 默认 `release.cert`（JSON）。
- `publish` 读插件目录里的 `nsgo.yaml`：`name` 必填；`id` 就是签进包里的 `App.ID`，要写成 `ai.yldm.kando.plugin.<publisher>.<name>`，不写会按 name 生成一个；`version` 默认 `0.0.0`；`build_number` 是防回滚比较的 build，不写按 version 推算；`capabilities` 用 snake_case，P3 只许写 `host` 和 `network`。`-entry` 不给时先找主包运行的 TSX 入口，再找 `App.tsx`。`-min-runtime` 默认是工具自己的 runtime 版本。对一个已有的 `.nsgopkg` 加 `-out` 是用另一把发布密钥重签。
- `revoke` 的 `-sequence` 每个根单调递增，Kando 拒收不比已有更新的列表（`ErrStale`）；`-days 0` 是永不过期，6.4 节按「过期只暂停联网和凭据类的非官方插件」处理，所以建议给有限期并随 appcast 续签。
- 运行时：根公钥作为字符串常量写进 Kando，每次启动先 `pkg.DecodePublicKey`（标准 base64，必须正好 32 字节），再 `trust.AddRoot`（6.4）；吊销列表用 `pkg.ParseRevocations(data)` 读，再 `trust.UpdateRevocations(list)`。
- 证书有效期拿包头里的 `CreatedAt` 比，不拿验证时的时间（`tsx/pkg/package.go:428`），所以有效期不能代替吊销（6.4）。

**5. 新发现的 NSGo 缺口**（spike 不提 issue，由主会话查重后提交）：

1. 第二个图没法持久化 Storage：`core.NewStore`（`core/storage.go:47`）要一个 `driver.StorageBackend`，它的 `Write` 参数是 `internal/driver.StorageChange`（`internal/driver/services.go:748-755`），app 实现不了；也没有导出的文件型 Store，拿不到 app 自己的 Store 给第二个图用。复现：`root := core.NewRoot(core.RootOptions{Post: post})`，在它上面 `tsx.Load` 一个插件，`Storage.setItem` 写进去的东西进程重启后就没了。
2. 没有 Tree 的图上 `tsx.Load` 返回 `runtime error: invalid memory address or nil pointer dereference`，原因是 tsx 给运行时建一个 region 元素（`tsx/tsx.go:148`），`core.NewElement` 要用 `cx.tree()`（`core/ops.go:25-28`）。要先 `core.NewTree(root, nil)`，文档没有写。
3. ErrBudget 不停掉插件，而预算只按最外层同步片段算（`tsx/sandbox.go:311-329`、`tsx/view.go:170-174`）：用 setTimeout 分片的调用永远不被切断，可以让引擎持续忙碌、内存无限上涨，`Limits` 里没有单次调用的总时长或 CPU 占比上限，也没有给宿主的 `Plugin.Interrupt`。复现：`export async function go() { for (;;) { const t = Date.now(); while (Date.now() - t < 450) {} await new Promise(r => setTimeout(r, 0)) } }`，在 `Limits.Call = 500ms` 下 Call，5 s 里引擎的空闲投递等待 p50 449 ms，done 一直不被调用。与 #760 相关，但这是时间上的缺口。
4. #759 的一个可行修法：tsx 在建运行时的时候给 regexp2 设 `MatchTimeout`（例如取 `Limits.Call`），不必等 goja 支持中断。Kando 先用进程级的 `regexp2.DefaultMatchTimeout` 绕开，可以作为评论补到 #759。
5. 链接 tsx 让二进制大约 37%，主要是 goja 和 tsx 里非常量的反射方法调用让链接器保留了所有导出方法（第 1 条）。不算 bug，可以作为体积问题报上去。
6. 文档小出入：`docs/public/zh/tsx.md:92` 说 `tsx.Load` 「在后台运行它」，实际上 `Load` 在调用方的 goroutine 上同步执行入口模块（`tsx/view.go:64-93`）。

**放行建议：有条件放行（go）**。正常插件的开销很小（Load 毫秒级，Call 微秒级，1 万次 host.call 期间输入延迟 p99 不到 2 ms），两个已知的洞加上新发现的分片问题都能由 Kando 自己兜到「最坏卡一个 500ms 片段、多占约 0.5 GB，然后插件被停用」。条件：

1. owner 接受可执行文件大 19–21 MiB（约 +37%）。
2. P3.1 起就带上三样东西，并有测试锁住：启动时设 `regexp2.DefaultMatchTimeout`；6.6 节的看门狗（全局串行、心跳、堆阈值、在片段之间 Close、记熔断）；不 fsync 的 in-flight 标记。测试夹具至少有：n=32 的回溯正则在 0.5 s 内失败，setTimeout 分片的 CPU 和内存插件在 2 s 内被停用，模块级数组反复调用在超出堆阈值后被停用。
3. owner 同意 `apps/kando/go.mod` 加 `github.com/dlclark/regexp2/v2`（直接依赖）和 `github.com/google/pprof`（indirect），这与 12.2 第 7 条字面上冲突（12.3 第 3 条）。
4. P3 的 CI 在 Windows 上也跑插件测试，这次没有量 Windows 的运行时。
5. 根密钥的保管（12.3 第 1 条）定下来之前，只跑官方插件和开发者模式插件。

如果 owner 不接受第 1 条的体积，就停在 P2；体积这一项不是 Kando 能削掉的，只能等上游。
- P2（2.0–2.5 已完成）：E5、E6、E7、5.1、5.3、6.2 已同步改过。和正文的出入如下：
  - P2.0：`ChatWith` 的 `ChatReply.Usage` 是 `media.ChatUsage{Prompt, Completion, Estimated}`（`Total()` 求和）；服务商不返回 usage 时用 `media.EstimateTokens`（ASCII 每 4 个字符算 1 个 token，其他字符每个算 1 个，宁高勿低）。额外请求头不能覆盖 Content-Type 和 Authorization。`Chat` 改成包一层 `ChatWith`，请求体逐字不变（仍带 `temperature: 0`，不带 `max_tokens`）。
  - P2.1：没有做成 `runTask`，只抽出共同的那一段 `aiAsk`（见 E6），各处的状态和解析留在原处；行为不变，由 `ai_ask_test.go` 和五处原有的测试锁住。
  - P2.2：task 包的「授权」就是插件页的开关（装好默认关），没有单独的 `kando.llm.chat` 勾选，因为数据包只有这一项能力；没有条目时 task 包是关的（`State.EnabledPack`，和 P3a 的脚本插件共用这个入口）。预算放在 `PluginState.Budget`（第 5.1 节原定 P3 才加），没有升 `State.Format`：旧版读到会忽略这个键，旧版也装不了 task 包，丢掉它最多是回到默认预算。用量存在新的 `usage.json`，不在 state.json 里，因为它每次运行都要写。入口是命令面板、「学习」菜单和播放器 ⋯ 菜单；侧栏「扩展」标签仍属 E15/P5。结果复用讲解这句的侧栏样式，但不支持追问和存进笔记。
  - P2.3：风格预设只是把包的 prompt 填进提示词，不另存选择（见 E5）。
  - P2.4：导出的选择存在 `savedPlugins.PluginExportTarget`；收藏句子面板的状态文字对模板导出只说写了几句、写到哪个文件，不给 Anki 的导入说明。文件菜单的「导出到 Anki…」仍叫这个名字，但按面板里选的方式导出。
  - P2.5：「文件」菜单和命令面板里有「把术语表导出为插件包…」和「把生词本导出为词表…」两行（插件块自己的 `pluginsKeys`，Key 为空，只在有可导出的内容时出现），打开主窗口上的一个表单（`settings.PluginExportDialog`，经插件块已有的 `pluginsOverlay` 画出，打开时占用 `holdModal`）：名称、id（预填 `local.<名称的 slug>`，没改过 id 时跟着名称变，名称是中文时用 `local.my-glossary`、`local.my-words`），术语表还有范围（全局、正在看的影片自己的、某个系列的）和原文、译文语言（预填正在看的影片或设置里的语言；用户自己的术语条目不带语言，所以由用户确认），词表还有语言（生词本里出现过的说话语言，一种语言一个包）。词表分两级：按复习进度，`Box` 达到 `study.MasteredBox` 的是「已掌握」，其余是「学习中」，等级名称按界面的五种语言写进包里。点「导出…」先在后台用 `plugins.Build` 把清单和文件写进临时文件夹、过一遍 `plugins.Validate`（和安装时一样），通过后打成 zip，再用系统的保存面板存成 `<id>.kandopack`；校验不过（比如名称和内置服务商重名）就在表单里说原因。包的 `license` 写 `Private`（用户自己的内容，不替用户选许可），版本是导出当天的日期。超过 200 字符的术语和超过 5000 条之后的术语不放进包，生词本里以 `#` 开头的词（词表文件里那是注释）也不放。导出的包可以直接用「从文件安装…」装回去，由 `plugins/export_test.go` 和根目录的两个测试锁住。
  - 截图场景 `TestScreenshotsPlugins` 加了 `plugins-task`（侧栏里 cards 形式的回答）、`plugins-translate-style`（设置 › 翻译 选中风格预设）、`plugins-export-template`（收藏句子面板的导出下拉框）和 `plugins-export-glossary`（导出术语表的表单），插件页那张多了一个 task 包。和 P1.4 一样，这些只在 Linux 上无头跑过、断言了节点，没有看过实际画面（已在 macOS 截图里看过，见下面「插件截图核对」）。
- 评审之后的修改（P2）：回答上限按形状算、超过上限的形状拒收；`ChatWith` 识别 `finish_reason: length`（`ErrReplyCutOff`，`Chat` 照旧返回已写出的文字）；发送前按预算估算、film-so-far 裁到预算放得下；发出后失败的请求按提示词估算计入预算；task 结果在侧栏被换掉时停下；没听到的句子不发送；卸载时清掉用量；`plugins.TermsMax`、`TermMax`、`TextOf` 和 `task.LineAt`、`task.Trim` 从 package main 挪进了非界面包；安装确认写出替换后保留的预算；模板导出的音频提醒。导出下拉框、风格预设下拉框、翻译来源下拉框和播放器「扩展」标签的面板下拉框原先因为 `ui.Select` 只接收静态的 Options 而在选项变化时重建（[nsgo-dev/nsgo#848](https://github.com/nsgo-dev/nsgo/issues/848)）；NSGo 升到 f377daba0a2a 后 `ui.Select` 有了 `OptionsFunc`，四个下拉框都改成绑定选项，包或面板变了时原地更新、焦点不丢。`ui.Select` 的 `Label`（读屏名称）仍然只读一次，所以四个下拉框各自还在一个只读界面语言的 `core.Dynamic` 里，切换语言时重建，代码里留了 `// TODO(nsgo#145): …`（[nsgo-dev/nsgo#145](https://github.com/nsgo-dev/nsgo/issues/145)）。
- P5 底座（E14，`refactor/media-stage-error`）：E14 已同步改过。和正文的出入如下：
  - `StageError` 比原定的 `{Stage, Err}` 多一个 `Source`，是失败的翻译来源的名字。`Engine` 的错误在 `media` 里先包一层不导出的 `engineError`（文字不变，`ClassifyTranslate` 照旧能认），`Make` 生成 `StageError` 时把名字取进 `Source`；只有 `Source` 会存进 library.json，所以读回来的失败也认得出来源。
  - 名字用 `Tools.EngineName` 字段给，`SegmentTranslator` 只有 `Translate` 一个方法（`ChatTranslator` 也满足它）；检查和离线判断用可选方法（`TranslateChecker`、`Local() bool`），不扩大接口。
  - 旧版 media 里从没有写过 `proofread:` 前缀（git 历史里没有），所以 `phonesync` 原来对 `proofread` 的分支删了，`ReadStageError` 也只认五个步骤名；其他前缀（例如 `yt-dlp: …`）和以前一样当作没有步骤。
  - 不在 `media.StageError` 里、却以步骤名开头的内存中的错误（只有测试会这样造）现在不再被当成那一步的失败。测试里原来用 `errors.New("download: …")` 造失败的地方改成 `media.ReadStageError("download: …")`，也就是「从 library.json 读回来的旧失败」，`fmt.Errorf("transcribe: %w", …)` 改成 `&media.StageError{…}`。
  - 界面只在 `Source`（或 `TranslationSource`）不为空时有变化，现有截图场景都没有这种失败，所以没加场景；改动的地方由 `TestAFailedCardNamesTheTranslationSource`、`TestAFilmNotTranslatedNamesTheTranslationSource` 和 `jobfailure` 的测试锁住。
  - 评审之后的修改：设了 `Engine` 而 AI 服务没配好时跳过校对、终校和术语建议（选了跳过，而不是在 `CheckTranslate` 里要求 AI 服务，因为只用机器翻译的人不该被迫配 AI 服务）；`Source` 只给来源自己的失败；离线拒绝也带来源名；`EngineName` 为空时用类型名；`Err` 为 nil 不 panic；`core.Job.TranslationSource`；卡片相关的纯函数挪进 `jobfailure`。包名没用 `failure`，因为 `films` 里已有同名函数。
- P5 翻译来源（E14，`feat/plugins-translators`）：E14、4.2、5.3 已同步改过。和正文、和 owner 的要求的出入如下：
  - `packages/media` 多导出了一个函数 `SpeechLanguageOf(ctx)`（原来不导出的 `speechLanguageOf`），给插件来源 `from`；除此之外没有改 media。`Make` 起草时的 ctx 里有用户给的或听出来的语言，借用站点字幕（`copied`）时没有，那时 `from` 为空，由来源自己判断。
  - DeepL 的 Free 和 Pro 主机不按密钥是否以 `:fx` 结尾来选：密钥不进 JS，插件看不到它。改成先在 `api-free.deepl.com` 上 POST `/v2/usage`，403 再试 `api.deepl.com`，记住接受这把密钥的主机，字幕只发往这一台；顺便在额度用完时直接按 `quota` 失败。DeepL 的 Free 密钥都以 `:fx` 结尾，所以 Free 密钥的请求从不到 Pro 主机；Pro 密钥第一次多一个 `/v2/usage` 的 403。换了密钥后，第一次翻译会在记住的旧主机上得到一个 403（DeepL 在处理正文前就拒绝），再重新找主机。
  - 翻译来源没有 `export` 字段，导出名固定（`translate`、`candidates`），同一个插件的多个来源靠 `context.translator` 区分。`local: true` 只给不申请网络和凭据的插件。
  - 适配器实现了 `TranslateChecker`（`fix/plugins-host-api` 补上）：`CheckTranslate` 没有 ctx、在任务的 goroutine 上同步调用，所以不读系统的安全存储，而是回引擎的 goroutine 看看看懂为详情页读到、改过的密钥（`pluginsScripts.secrets`，`pluginsNoCredential`）。插件声明了凭据而一把密钥都没存时以 `no-credential` 失败，任务在发出任何一句前就说「还没有密钥，去插件里填写」；没有凭据、密钥还没读到、系统没有安全存储（`noSecrets`，这时插件本来就不启动，翻译照旧以 `not-running` 失败，不让用户去填一把存不下的密钥）、或引擎 5 s 内没回答时放行，翻译时照旧失败。`TestOfficialDeepL` 和 `TestPluginTranslatorTranslatesTheFilms` 锁住。
  - 插件来源翻译一个 cell 时进度只在 cell 之间动：`media` 的进度上报（`reportProgress`）不导出，`Engine` 报不了批次进度。没有为此再改 media。
  - ⇧E 只在按下时插件已载入、并导出了 `candidates` 时问插件，否则问 AI 服务（和没选插件时一样，没有 AI 服务就说没有）。DeepL 插件的候选是同一句在 `formality` 为 `prefer_more` 和 `prefer_less` 下的两种译法，前后几句原文作为 DeepL 的 `context` 一并发送，由看懂去掉和现在的译文相同的、重复的。
  - 评审之后的修改：启动时恢复的任务改成在翻译开始时才确定插件、等它载入（原来在 `queuePipeline` 时插件目录还没读，`Engine` 为空，会悄悄用 AI 服务翻译）；跑不起来的插件不再算在用，而是按 `not-running` 失败并在侧栏说明；状态变化时重查侧栏状态行和翻译提议；离线判断看来源的 `local`；whisper 的 `no` 换成 `nb`；等待中任务被停下不调插件；AI 服务没配好时不显示终校行；`jobfailure.SourceHint` 对离线拒绝给自己的话和隐私页；跟随移到插件块的 hooks。都有测试（`TestPluginTranslatorAtLaunch`、`TestPluginTranslatorFollowsReadinessAndOffline`、`TestOfficialDeepL` 末尾没有安全存储的部分、`TestSpokenTag`、`TestSourceHintOffline`）。
  - 失败卡片的按钮：带「打开设置」的失败卡片有三个按钮（打开设置、重试、移除），在英文里第三个、有时第二个会折到第二行，main 上 AI 服务的失败卡片也一样。现在所有失败和取消的卡片都把「移除」做成右上角的 ×（和排队、运行中的卡片的 × 同一个位置），下面一行只放能修好它的按钮、重试和报告；卡片文字栏原来整栏给 × 让出 18 pt，现在只有标题那一行让出。截图场景 `plugins-translate-failed`（以及 `-en`、`-ja`）断言这三张失败卡片的按钮在同一行、不超出卡片。
  - 截图场景 `plugins-translate-source` 用仓库里的 DeepL 插件（标题「DeepL」），不用测试夹具。
  - 来源不在了不清掉存着的选择：插件重新打开后又用回它；在那之前任务用 AI 服务，设置页说明，消息栏每次运行说一次（`prefs.sourceGoneSaid`）。设置页的下拉框列出已启用（不一定已载入）的插件来源，免得启动验签期间选项闪烁。
  - 失败卡片：P5 底座定的是来源失败一律不给「打开设置」；带码的失败现在给：`auth`、`no-credential`、`not-running` 到插件页，`language` 到翻译页，因为那就是能修好它的地方。没有码的失败照旧不给。
  - 播放器进度徽章的「翻译」一行写来源的标题，「终校」一行仍写 AI 服务的模型。
  - 官方插件的源码按 P3c 的约定放在 `apps/kando/plugins/official/deepl/`（本分支起初放在仓库根目录 `plugins/official/kando.deepl/`，P3c 合并后挪了过来），有 `nsgo.yaml`，所以 `make official-plugins` 不带参数时也签它；`deepl` 包的 `TestPluginDeepLPublishes` 检查 `nsgo.yaml` 签的 App ID、版本和权限就是清单声明的（只有 `kando.net.credentialFetch`，没有网络规则），凭据只发往 DeepL 的两台主机。清单写 `"kando": ">=0.37.0"`：`translators` 随 #329 合并时 v0.36.0 已经发布，第一个认得它的是 v0.37.0。AnkiConnect 写 `>=0.36.0`，因为导出期间读声音不占每分钟额度、表格显示插件自己的失败原因都随 #328 进了 v0.36.0，更早的版本超过 60 句会丢声音。`kandoManifest()` 用 esbuild 的 JSON import 直接返回清单里的 `capabilities`、`contributes`、`credentials`，不再手抄一份。根目录的 `TestOfficialDeepL` 和截图场景按发布的做法用临时根签名、`official.Wrap` 打包、再安装 `.kandoplugin`（`deeplRelease`），插件文件夹用 `runtime.Caller` 从测试源文件的位置找，因为 `nsgo test -target` 不在包目录里运行测试。本分支没有用官方发布密钥签名；之后 owner 用 `make official-plugins` 签好 1.0.0，放进 kando-plugins 的 `scripts/kando.deepl/`（连同 `src/`），已列在社区索引里（P3c）。
  - 测试：`plugins/translators_test.go`（清单规则、`Takes`、回答解析、错误码），`plugins/script` 的 `TestScriptProbeTranslators`，`jobfailure` 的 `TestSourceHintOfACode`（五种界面语言里每个码都有一句不同的、带来源名的话），根目录 `plugins_translate_test.go`：来源的提供、保存、任务用它翻译（120 行分 50/50/20 三批、顺序不变）、`quota`/`bad-reply`/`language`、来源不在了改用 AI 服务并说明、⇧E 问插件；`TestOfficialDeepL` 用临时根打包仓库里的 DeepL 插件，接 httptest 的假 DeepL（`credentialFetch` 的 client 换成拨到测试服务器的，和 P3b 一样）：没有密钥、Free 和 Pro 主机、语言代码（ZH-HANS、ZH-HANT、EN-US、EN-GB、PT-BR）、空行不发、行数不变、456 和额度用完是 `quota`、两台都 403 是 `auth`，并检查密钥只出现在 Authorization 头里，不在插件组的请求体和插件的回答里，插件代码不碰 `Secrets`、`Storage`。截图场景 `plugins-translate-source`（设置 › 翻译 选了插件来源）和 `plugins-translate-failed`（DeepL 不接受密钥的失败卡片）只在 Linux 上跑了无头断言，没有出图。
- 宿主接口补漏（`fix/plugins-host-api`）：5.1、5.3、5.4 已同步改过。
  - 控制 socket 的只读 `plugins.list`（`remote.MethodPlugins`）补上了，被锁的资料库照样拒绝（`TestLockedLibraryLeaksNothing`），回答里没有目录（`TestRemoteListsThePlugins`）。
  - 影片 id：原来 `kando.film.current`、`film.opened`、`job.completed` 拿磁盘上的绝对路径派生，`kando.saved.read`、`kando.words.read` 拿库内相对路径派生，同一部影片在同一个插件眼里是两个 id；现在都先换成库内相对路径（`study.KeyIn`，`study.Book.Key` 也改成调它）。`film.opened` 的 `spoken` 改成和 `kando.film.current` 一样回落到用户指定的语言。
  - `kando plugin install` 改用 `publish.Check`，和 `validate` 一样验签，开发者插件在命令行里装不了（`TestPluginValidateScript`）。
  - 测试补上（`plugins_host_test.go`）：没有影片时 `film.current` 是 null、`lines.seen` 是空数组；有影片时 `film.current` 的形状、`film.opened` 和收藏句子、生词里的影片 id 一致，两个插件的 id 不同、Host 不同（第 9 节「每个插件拿到的 `*core.Host` 都是不同的实例」）；`lines.seen` 和 `line.current` 不越过正在播放的这一句；`sound.read` 的 `{current: true}`；被锁的资料库拒绝所有 `Perm.Library()` 的权限、`kando.toast` 照常；`llm.changed` 推到；对除 `kando.ui.openURL` 外的每个 handler 按表调用一次，回答里没有 AI 服务的地址和 key、资料库的路径、`baseURL`/`apiKey` 这样的字段（第 9 节的不变量，`kando.ui.openURL` 只回是否打开，由 `TestPluginScriptOpenURLWaitsForTheUser` 测）。`plugins/filmid_test.go` 测 `FilmID`。`TestPluginHostLLMChatRefusals` 测 `llm.chat` 没有 AI 服务时的 `unavailable`、回答被截断时的 `cut-off`、上一个请求还在等时的 `busy`；离线时云端服务的 `llm.chat` 在授权求交时就被去掉，回的是 `denied`，不会走到 `unavailable`。
  - 没有界面文字的改动，截图场景没有变化。
- 插件截图核对（`fix/plugins-screenshot-review`）：下载了 main 上 CI 运行 44637826099（提交 `7954bc5`）的 `screenshots-macos`，逐张看过全部插件场景：`settings-plugins`、`settings-plugins-dictionaries`、`plugins-install`、`plugins-install-site`、`plugins-lookup`、`plugins-marks`、`plugins-task`、`plugins-translate-style`、`plugins-export-template`、`plugins-export-glossary`、`plugins-script-install`、`plugins-script-install-official`、`plugins-script-details`、`plugins-script-lookup`、`plugins-script-subtitle-format`、`plugins-script-exporter`、`plugins-script-credential`、`plugins-script-marks`、`plugins-script-panel`、`plugins-translate-source`、`plugins-translate-failed`（含 `-en`、`-ja`）、`plugins-index`、`plugins-updates`。上面各条里「没有看过实际画面」「没出图」的场景都在这次看过了。改了下面这些，E2 已同步改过：
  - 读音 view 的竖直对齐：带读音的字比旁边的字高约 2.5 pt（同一个「の」在 `山の上` 的读音 view 里比在 `どの` 里高 2–3 个像素），读音 view 里的等级线、虚线也比同一个词其余部分的线高 2–3 个像素，一个词下面的线断成高低两截。原因是 NSGo 按行字体（系统字体）的 descent 放 `VerticalAlignTextBottom` 的 attachment（`internal/driver/text.go` 的 `TextAttachment.Ascent`），而读音 view 里的字回退到 CJK 字体，它的 descent 更大，Apple 上的行盒还在基线下面多半个 leading；NSGo 只有 Em canvas 能把 attachment 自己的基线放到行的基线上（`core/commit.go` 的 `baselineDescent`），装着文字的 view 不行。看懂里的绕行：读音 view 里那段原文的 `OnLayout` 和 `OnTextLayout` 量出它的盒子底在基线下面多远（`film.rubyDrop`），这段原文按这个距离挂到 view 下面（负的下边距），view 改用 `VerticalAlignBaseline`，于是 view 的底就是这段原文的基线、落在行的基线上；量到之前照旧用 `VerticalAlignTextBottom`。线由这段原文按自己的基线画，所以跟着对齐。上游还没有 issue，`rubyView` 里留了 `TODO(nsgo)`，提了 issue 后补上号和链接。Linux 上 nsgotest 的假排版没有 descent，这个差在无头测试里看不出，只测了 `rubyDrop` 的算法（`TestRubyDrop`），改后的画面要在 macOS 截图任务里重看 `plugins-marks`。
  - 描边副本：`plugins-marks` 的画面是黑的（假视频不出画面，`TODO(nsgo#107)`），黑色描边和模糊在黑底上看不出，读音 view 上的模糊滤镜是否生效仍不能从截图判断；描边副本和最上面那份用同样的读音 view 和同样的量法，排版一致。
  - 假名注音盖住了词中间的假名：IPA 词典把 `山の上` 当一个词，读音 `やまのうえ` 整个盖在 `山の上` 上。`furigana` 现在也在词中间的假名处切开：读音里在相应位置有同样的假名、而且只有一种对法时，`山` 标 `やま`、`上` 标 `うえ`（`取り扱い` 是 `取` 标 `と`、`扱` 标 `あつか`），对不上或对法不止一种时照旧整段一个读音（E2）。复核时发现第一版按「每段汉字取最短的读音」挑第一种对法，`物の怪` 读 `もののけ` 会标成 `物` 读 `も`、`怪` 读 `のけ`，改成只在对法唯一时切开（`TestFurigana` 有这一例）。
  - 截图里的插件名：P3b 的 `acme.rivers` 和 E15 的 `acme.study` 用的是测试默认的「Anki 发送器」，查词卡、收藏句子的导出结果、详情页和扩展标签都写着这个名字；官方 `kando.ankiconnect` 的安装确认写着「由看懂发布」，作者却是 Acme。测试夹具加了 `pluginScriptFolderAbout`，这三个插件各有自己的名称、说明和作者（河流学习包、台词精读，以及和 `plugins/official/ankiconnect` 相同的推送到 Anki、Kando）。
  - 看过没改的：`plugins-marks` 里 `山` 在词表里却没有画线，因为分词把 `山の上` 当一个词，词表按整词查（按词里的部分查词表是另一个改动）；`plugins-translate-failed-en` 的失败原因两行放不下，后半句「Check it in the plugin's details」被截掉（失败原因本来最多两行，别的失败也一样）；`plugins-script-exporter` 底部的导出结果挤在下拉框左边折成两行。其余场景没有发现错位、截字、和主题不符的颜色或重叠。手机端 `apps/mobile/player/marks.go` 的读音 view 也是 `VerticalAlignTextBottom`，应有同样的错位，这次没改（手机截图不在这次核对的范围）。
  - 要重新出图的场景：`plugins-marks`（对齐和注音）、`plugins-script-lookup`、`plugins-script-exporter`、`plugins-script-credential`、`plugins-script-panel`、`plugins-script-install-official`（插件名）。
