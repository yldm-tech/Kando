package account

import (
	"strings"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// App is the account page. It draws and reports edits. It does not sign in or upload by itself.
type App interface {
	GoogleEmail() string
	GoogleBusy() bool
	GoogleErr() string
	GoogleConfigured() bool
	SignInGoogle()
	SignOutGoogle()
	Storage() *string
	StorageKind() string
	R2Endpoint() *string
	R2Bucket() *string
	R2AccessKey() *string
	R2Secret() *string
	S3Region() *string
	S3Bucket() *string
	S3AccessKey() *string
	S3Secret() *string
	OSSRegion() *string
	OSSBucket() *string
	OSSAccessKey() *string
	OSSSecret() *string
	COSRegion() *string
	COSBucket() *string
	COSAccessKey() *string
	COSSecret() *string
	CompatEndpoint() *string
	CompatRegion() *string
	CompatBucket() *string
	CompatAccessKey() *string
	CompatSecret() *string
	CompatPathStyle() *bool
	ShowR2Secret() bool
	ToggleR2Secret()
	Save() error
}

// Show draws the account settings page.
func Show(c *ui.Context, t *ui.Theme, a App) {
	section(c, t, "Google", "登录只用来标明是谁上传的。刷新令牌保存在本机设置文件里。", func() {
		card(c, t, func() { googleRow(c, t, a) })
	})
	section(c, t, "对象存储", "视频不会在下载完成时自动上传。到播放页点上传。各处的密钥分开保存，只存在本机设置文件里。", func() {
		switch a.StorageKind() {
		case "s3":
			card(c, t,
				func() { storageRow(c, t, a) },
				func() { textRow(c, t, "s3-region", "Region", "例如 ap-northeast-1。", a.S3Region(), a) },
				func() { textRow(c, t, "s3-bucket", "Bucket", "", a.S3Bucket(), a) },
				func() { textRow(c, t, "s3-access", "Access Key", "", a.S3AccessKey(), a) },
				func() { secretRow(c, t, "s3-secret", a.S3Secret(), a) },
			)
		case "oss":
			card(c, t,
				func() { storageRow(c, t, a) },
				func() {
					textRow(c, t, "oss-region", "Region", "例如 cn-hangzhou。内网填 cn-hangzhou-internal。", a.OSSRegion(), a)
				},
				func() { textRow(c, t, "oss-bucket", "Bucket", "", a.OSSBucket(), a) },
				func() { textRow(c, t, "oss-access", "Access Key", "", a.OSSAccessKey(), a) },
				func() { secretRow(c, t, "oss-secret", a.OSSSecret(), a) },
			)
		case "cos":
			card(c, t,
				func() { storageRow(c, t, a) },
				func() { textRow(c, t, "cos-region", "Region", "例如 ap-guangzhou。", a.COSRegion(), a) },
				func() {
					textRow(c, t, "cos-bucket", "Bucket", "要带 APPID，例如 video-1250000000。", a.COSBucket(), a)
				},
				func() { textRow(c, t, "cos-access", "Access Key", "填 SecretId。", a.COSAccessKey(), a) },
				func() { secretRow(c, t, "cos-secret", a.COSSecret(), a) },
			)
		case "compat":
			card(c, t,
				func() { storageRow(c, t, a) },
				func() {
					textRow(c, t, "compat-endpoint", "Endpoint", "完整的 https 地址。", a.CompatEndpoint(), a)
				},
				func() {
					textRow(c, t, "compat-region", "Region", "签名用的区域名。MinIO 通常填 us-east-1。", a.CompatRegion(), a)
				},
				func() { textRow(c, t, "compat-bucket", "Bucket", "", a.CompatBucket(), a) },
				func() { textRow(c, t, "compat-access", "Access Key", "", a.CompatAccessKey(), a) },
				func() { secretRow(c, t, "compat-secret", a.CompatSecret(), a) },
				func() { pathStyleRow(c, t, a) },
			)
		default:
			card(c, t,
				func() { storageRow(c, t, a) },
				func() { endpointRow(c, t, a) },
				func() { textRow(c, t, "r2-bucket", "Bucket", "", a.R2Bucket(), a) },
				func() { textRow(c, t, "r2-access", "Access Key", "", a.R2AccessKey(), a) },
				func() { secretRow(c, t, "r2-secret", a.R2Secret(), a) },
			)
		}
	})
}

func storageRow(c *ui.Context, t *ui.Theme, a App) {
	fieldRow(c, t, "位置", "", func() {
		c.Key("storage-kind")
		selected := a.Storage()
		// The drop-down shows the storage names; the setting keeps the id.
		label := storageLabel(*selected)
		labels := make([]string, len(storageKinds))
		for i, id := range storageKinds {
			labels[i] = storageLabel(id)
		}
		if breeze.Select(c, &label, labels).Changed() {
			for _, id := range storageKinds {
				if storageLabel(id) == label {
					*selected = id
				}
			}
			_ = a.Save()
		}
	})
}

var storageKinds = []string{"r2", "s3", "oss", "cos", "compat"}

func storageLabel(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "s3":
		return "Amazon S3"
	case "oss":
		return "阿里云 OSS"
	case "cos":
		return "腾讯云 COS"
	case "compat":
		return "S3 兼容"
	default:
		return "Cloudflare R2"
	}
}

func pathStyleRow(c *ui.Context, t *ui.Theme, a App) {
	fieldRow(c, t, "路径样式", "MinIO 打开。B2、Wasabi、华为云、七牛保持关闭。", func() {
		c.Key("compat-path")
		before := *a.CompatPathStyle()
		breeze.Switch(c, a.CompatPathStyle()).Label("路径样式")
		if *a.CompatPathStyle() != before {
			_ = a.Save()
		}
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
			if breeze.Button(c, "退出").Shrink(0).Clicked() {
				a.SignOutGoogle()
			}
			return
		}
		label := "使用 Google 登录"
		if a.GoogleBusy() {
			label = "正在登录…"
		}
		c.Key("google-sign-in")
		btn := breeze.PrimaryButton(c, label).Shrink(0).Disabled(a.GoogleBusy() || !a.GoogleConfigured())
		if a.GoogleConfigured() && !a.GoogleBusy() && btn.Clicked() {
			a.SignInGoogle()
		}
	})
}

func endpointRow(c *ui.Context, t *ui.Theme, a App) {
	fieldRow(c, t, "Endpoint", "填 Account ID，或完整的 https 地址。", func() {
		c.Key("r2-endpoint")
		input := fit(breeze.TextInput(c, a.R2Endpoint()).Label("Endpoint"))
		if input.Changed() {
			_ = a.Save()
		}
	})
}

func textRow(c *ui.Context, t *ui.Theme, key, label, hint string, value *string, a App) {
	fieldRow(c, t, label, hint, func() {
		c.Key(key)
		input := fit(breeze.TextInput(c, value).Label(label))
		if input.Changed() {
			_ = a.Save()
		}
	})
}

func secretRow(c *ui.Context, t *ui.Theme, key string, value *string, a App) {
	fieldRow(c, t, "Secret", "", func() {
		c.Key(key)
		input := fit(breeze.TextInput(c, value).Label("Secret"))
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
		if breeze.Button(c, label).Shrink(0).Clicked() {
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
	ui.Themed(c, cardTheme(t)).Radius(12).Padding(4, 16).Children(func() {
		for i, line := range lines {
			if i > 0 {
				ui.Box(c).Fill().Height(1).Background(t.Border)
			}
			line()
		}
	})
}

// cardTheme is the theme inside a card: breeze draws a control's face as a tint of the background behind it, so the card's color takes the place of the window's.
func cardTheme(t *ui.Theme) *ui.Theme {
	card := *t
	card.Background = t.Surface
	return &card
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
