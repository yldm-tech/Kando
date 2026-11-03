package account

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// App is the account page. It draws and reports edits. It does not sign in or upload by itself.
type App interface {
	GoogleEmail() string
	GoogleBusy() bool
	GoogleErr() string
	GoogleConfigured() bool
	SignInGoogle()
	SignOutGoogle()
	R2Endpoint() *string
	R2Bucket() *string
	R2AccessKey() *string
	R2Secret() *string
	ShowR2Secret() bool
	ToggleR2Secret()
	Save() error
}

// Show draws the account settings page.
func Show(c *ui.Context, t *ui.Theme, a App) {
	section(c, t, "Google", "登录只用来标明是谁上传的。刷新令牌保存在本机设置文件里。", func() {
		card(c, t, func() { googleRow(c, t, a) })
	})
	section(c, t, "Cloudflare R2", "视频不会在下载完成时自动上传。到播放页点「上传到 R2」。密钥只保存在本机设置文件里。", func() {
		card(c, t,
			func() { endpointRow(c, t, a) },
			func() { textRow(c, t, "r2-bucket", "Bucket", "", a.R2Bucket(), a) },
			func() { textRow(c, t, "r2-access", "Access Key", "", a.R2AccessKey(), a) },
			func() { secretRow(c, t, a) },
		)
	})
}

func googleRow(c *ui.Context, t *ui.Theme, a App) {
	ui.Row(c).MinHeight(52).Padding(8, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			if email := a.GoogleEmail(); email != "" {
				ui.Text(c, email).Selectable()
			} else {
				ui.Text(c, "未登录")
			}
			switch {
			case a.GoogleErr() != "":
				ui.Text(c, a.GoogleErr()).TextColor(t.Danger).FontSize(12).Wrap()
			case !a.GoogleConfigured():
				ui.Text(c, "登录需要环境变量 LINGOCUT_GOOGLE_CLIENT_ID。Client ID 不写入设置文件。").TextColor(t.TextMuted).FontSize(12).Wrap()
			}
		})
		if a.GoogleEmail() != "" {
			c.Key("google-sign-out")
			if components.Button(c, "退出", components.ButtonSecondary).Shrink(0).Clicked() {
				a.SignOutGoogle()
			}
			return
		}
		label := "使用 Google 登录"
		if a.GoogleBusy() {
			label = "正在登录…"
		}
		c.Key("google-sign-in")
		btn := components.Button(c, label, components.ButtonPrimary).Shrink(0).Disabled(a.GoogleBusy() || !a.GoogleConfigured())
		if a.GoogleConfigured() && !a.GoogleBusy() && btn.Clicked() {
			a.SignInGoogle()
		}
	})
}

func endpointRow(c *ui.Context, t *ui.Theme, a App) {
	fieldRow(c, t, "Endpoint", "填 Account ID，或完整的 https 地址。", func() {
		c.Key("r2-endpoint")
		input := fit(components.TextBox(c, a.R2Endpoint(), components.TextBoxOptions{}).Label("Endpoint"))
		if input.Changed() {
			_ = a.Save()
		}
	})
}

func textRow(c *ui.Context, t *ui.Theme, key, label, hint string, value *string, a App) {
	fieldRow(c, t, label, hint, func() {
		c.Key(key)
		input := fit(components.TextBox(c, value, components.TextBoxOptions{}).Label(label))
		if input.Changed() {
			_ = a.Save()
		}
	})
}

func secretRow(c *ui.Context, t *ui.Theme, a App) {
	fieldRow(c, t, "Secret", "只保存在本机设置文件里。", func() {
		c.Key("r2-secret")
		input := fit(components.TextBox(c, a.R2Secret(), components.TextBoxOptions{}).Label("Secret"))
		if !a.ShowR2Secret() {
			input.Password()
		}
		if input.Changed() {
			_ = a.Save()
		}
		c.Key("r2-secret-toggle")
		label := "显示"
		if a.ShowR2Secret() {
			label = "隐藏"
		}
		if components.Button(c, label, components.ButtonSecondary).Shrink(0).Clicked() {
			a.ToggleR2Secret()
		}
	})
}

func section(c *ui.Context, t *ui.Theme, title, note string, body func()) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title)
		body()
		if note != "" {
			ui.Text(c, note).TextColor(t.TextMuted).FontSize(12).Wrap()
		}
	})
}

func card(c *ui.Context, t *ui.Theme, lines ...func()) {
	ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 16).Children(func() {
		for i, line := range lines {
			if i > 0 {
				ui.Box(c).Fill().Height(1).Background(t.Border)
			}
			line()
		}
	})
}

func fieldRow(c *ui.Context, t *ui.Theme, label, hint string, control func()) {
	ui.Row(c).Wrap().MinHeight(52).Padding(10, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label)
			if hint != "" {
				ui.Text(c, hint).TextColor(t.TextMuted).FontSize(12).Wrap()
			}
		})
		control()
	})
}

func fit(e ui.Element) ui.Element {
	return e.MinWidth(120).MaxWidth(360).Basis(220)
}
