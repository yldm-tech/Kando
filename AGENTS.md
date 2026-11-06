# LingoCut

界面按组件拆开，不要把一块功能全塞进一个文件。

- 一个界面一个目录，放在 `apps/lingocut/` 下面。目录名就是这块界面的名字，例如 `sidebar/`、`films/`、`film/`、`settings/`、`account/`。`view.go` 只负责把窗口壳和各页面接起来。`shelf/` 是任务记录，不是界面。`brand/` 是访达图标。`cloud/` 是 Google 登录和 R2 上传，不是界面。
- 新增一块，或改到已经堆在 `view.go` 里的一块，就新建文件并把那一块挪出去，不要在原地追加。
- 控件用 breeze-ui（`github.com/yldm-tech/mygo-breeze-ui/ui/breeze`，文档 `docs/breeze-ui.md`）：按钮、输入、开关、选择、侧栏、设置行、弹窗、提示。breeze 没有的先判断：可复用的平台控件给 breeze-ui 提 PR；只属于 LingoCut 的视觉（播放画面、字幕层、毛玻璃面、hero、卡片）用 MyGo 的布局原语自己画。不要再手写一套 breeze 已有的按钮或输入框。图标用各界面目录里的 `icons.go`（`ui.ParseSVG`），breeze 不带图标。
- 组件只负责画和把点击、输入交回去。业务状态留在 app 和 nodra session 里。构建界面的函数里不要读盘、不要发请求、不要起 goroutine。
- 一块的代码只写在它自己的文件里，不往共享文件里追加：状态写在 `app` 上属于它的那个字段，类型放在 `state_<area>.go`（队列在 `queue.go`），`main.go` 的 `app` 只留各块共用的字段；存进 settings.json 的设置写进它自己的 `saved<Area>`（已经嵌在 `settings_file.go` 的 `savedSettings` 里，键不能和别人重名）；启动恢复、窗口就绪、后台更新后、每帧之后、切换资料库、书架或组件、AI 服务变化时要做的事，写进它自己的 `<area>Hooks()`（`events.go` 的 `areaHooks`），不去 `main`、`view` 或别的块的函数里插调用；快捷键写在它的 `<area>Keys()`；测试放 `<area>_test.go`，公共辅助在 `helpers_test.go`；截图场景是 `screenshot_<area>_test.go` 里的 `TestScreenshots<Area>`，用 `screenshot_helpers_test.go` 的样例库。两块都用的读写方法放在状态所属那块的文件里，只属于一块的辅助函数名带上这块的前缀，免得在 `package main` 里撞名。
- 按界面切开就够了，不要为了拆再套一层抽象。
