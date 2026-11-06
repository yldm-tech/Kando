# LingoCut

界面按组件拆开，不要把一块功能全塞进一个文件。

- 一个界面一个目录，放在 `apps/lingocut/` 下面。目录名就是这块界面的名字，例如 `sidebar/`、`films/`、`film/`、`settings/`、`account/`。`view.go` 只负责把窗口壳和各页面接起来。`shelf/` 是任务记录，不是界面。`brand/` 是访达图标。`cloud/` 是 Google 登录和 R2 上传，不是界面。
- 新增一块，或改到已经堆在 `view.go` 里的一块，就新建文件并把那一块挪出去，不要在原地追加。
- 控件用 breeze-ui（`github.com/yldm-tech/mygo-breeze-ui/ui/breeze`，文档 `docs/breeze-ui.md`）：按钮、输入、开关、选择、侧栏、设置行、弹窗、提示。breeze 没有的先判断：可复用的平台控件给 breeze-ui 提 PR；只属于 LingoCut 的视觉（播放画面、字幕层、毛玻璃面、hero、卡片）用 MyGo 的布局原语自己画。不要再手写一套 breeze 已有的按钮或输入框。图标用各界面目录里的 `icons.go`（`ui.ParseSVG`），breeze 不带图标。
- 组件只负责画和把点击、输入交回去。业务状态留在 app 和 nodra session 里。构建界面的函数里不要读盘、不要发请求、不要起 goroutine。
- 一块的代码只写在它自己的文件里。新增一块时，共享文件里只加这几行，别的共享文件都不碰：`main.go` 的 `app` 一个字段和 `newApp` 一行，`settings_file.go` 的 `savedSettings` 一个嵌入和 `clean` 一行，`events.go` 的 `areas()` 一行，`keys.go` 的 `shortcuts()` 一项。状态写在 `app` 上属于它的那个字段，类型和构造函数放在 `state_<area>.go`（队列在 `queue.go`），`main.go` 的 `app` 只留各块共用的字段；存进 settings.json 的设置写进它自己的 `saved…` 部分，每个部分带自己的 `clean()`，键不能和别人重名，也不能有自己的 `MarshalJSON`/`UnmarshalJSON`（`settings_file_test.go` 会查）。一块可以把设置拆成几个部分放在自己的几个文件里，例如设置页的 `savedSpeech`、`savedLLM`、`savedTranslate`，书架的 `savedShelf` 在 `shelf_app.go`。启动恢复、读完 library.json、窗口就绪、后台更新后、每帧之后、切换资料库、书架或工具（ffmpeg、whisper）变化、AI 服务改了或测试过、续播位置变了时要做的事，写进它自己的 `<area>Hooks()`（`events.go` 的 `areaHooks`，那里写了启动时的先后：`restore`、`shelfLoaded`、第一次 `shelfChanged` 都在窗口出现之前，这时不能调 `update`），不去 `main`、`view` 或别的块的函数里插调用；快捷键写在它的 `<area>Keys()`；测试放 `<area>_test.go`（账号和上传是 `account_test.go`），公共辅助在 `helpers_test.go`；截图场景是 `screenshot_<area>_test.go` 里的 `TestScreenshots<Area>`，用 `screenshot_helpers_test.go` 的样例库。两块都用的读写方法放在状态所属那块的文件里，只属于一块的辅助函数名带上这块的前缀（例如 `libraryEnqueue`、`queueAdd`、`playerSaveSettings`），免得在 `package main` 里撞名。
- 跨块的约定以底座为准：要打开设置就调 `OpenSettings(section)`，判断设置开没开、开在哪一节看 `a.prefs.open` 和 `a.prefs.section`（测试用 `wantSettings`），不看 `a.page`；测试要进某一节设置就用 `settings_helpers_test.go` 的 `settingsOpenAt(a, tt, section)`，在它返回的 Tester 上点和断言，不去点侧栏；任务按个控制，页面用 `Cancel(id)` 和 `RunningJob()`，不再有 `Running()`/`Stop()`；加任务的公共入口是 `Enqueue(sources...)`；弹出模态对话框（文件面板、sheet、alert）时用 `holdModal()` 拿到 release，关掉时调用它，菜单栏用 `modalOpen()` 判断要不要让命令等着。
- 按界面切开就够了，不要为了拆再套一层抽象。
