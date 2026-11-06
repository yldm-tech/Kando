# LingoCut

界面按组件拆开，不要把一块功能全塞进一个文件。

- 一个界面一个目录，放在 `apps/lingocut/` 下面。目录名就是这块界面的名字，例如 `sidebar/`、`films/`、`film/`、`settings/`、`account/`。`view.go` 只负责把窗口壳和各页面接起来。`shelf/` 是任务记录，不是界面。`brand/` 是访达图标。`cloud/` 是 Google 登录和 R2 上传，不是界面。
- 新增一块，或改到已经堆在 `view.go` 里的一块，就新建文件并把那一块挪出去，不要在原地追加。
- 控件用 breeze-ui（`github.com/yldm-tech/mygo-breeze-ui/ui/breeze`，文档 `docs/breeze-ui.md`）：按钮、输入、开关、选择、侧栏、设置行、弹窗、提示。breeze 没有的先判断：可复用的平台控件给 breeze-ui 提 PR；只属于 LingoCut 的视觉（播放画面、字幕层、毛玻璃面、hero、卡片）用 MyGo 的布局原语自己画。不要再手写一套 breeze 已有的按钮或输入框。图标用各界面目录里的 `icons.go`（`ui.ParseSVG`），breeze 不带图标。
- 组件只负责画和把点击、输入交回去。业务状态留在 app 和 nodra session 里。构建界面的函数里不要读盘、不要发请求、不要起 goroutine。
- 按界面切开就够了，不要为了拆再套一层抽象。
