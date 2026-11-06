package account

import (
	"strings"

	"kando/apps/kando/settings/form"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// App is the 账号与上传 category of the settings window. It draws and reports edits. It does not sign in or upload by itself.
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
	Save() error
}

// Show draws the 账号与上传 category: the Google account uploads are filed under, and the object storage they go to, laid out as the prototype's page.
func Show(c *ui.Context, t *ui.Theme, a App) {
	form.Label(c, "账号", 6, nil)
	form.Card(c, func() { googleRow(c, a) })
	if a.GoogleErr() != "" {
		ui.Text(c, a.GoogleErr()).FontSize(11).TextColor(t.Danger).Wrap().Padding(0, 12)
	}
	note := "登录只用来标明是谁上传的。刷新令牌保存在本机设置文件里。"
	if !a.GoogleConfigured() && a.GoogleEmail() == "" {
		note = "需要设置环境变量 LINGOCUT_GOOGLE_CLIENT_ID；没有设置时登录按钮不可用。Client ID 不写入设置文件。"
	}
	form.Note(c, note, 10)
	form.Label(c, "对象存储", 6, nil)
	form.Card(c, func() {
		storageRow(c, a)
		switch a.StorageKind() {
		case "s3":
			textRow(c, "s3-region", "Region", "例如 ap-northeast-1。", a.S3Region(), a)
			textRow(c, "s3-bucket", "Bucket", "", a.S3Bucket(), a)
			textRow(c, "s3-access", "Access Key", "", a.S3AccessKey(), a)
			secretRow(c, "s3-secret", "", a.S3Secret(), a)
		case "oss":
			textRow(c, "oss-region", "Region", "例如 cn-hangzhou。内网填 cn-hangzhou-internal。", a.OSSRegion(), a)
			textRow(c, "oss-bucket", "Bucket", "", a.OSSBucket(), a)
			textRow(c, "oss-access", "Access Key", "", a.OSSAccessKey(), a)
			secretRow(c, "oss-secret", "", a.OSSSecret(), a)
		case "cos":
			textRow(c, "cos-region", "Region", "例如 ap-guangzhou。", a.COSRegion(), a)
			textRow(c, "cos-bucket", "Bucket", "要带 APPID，例如 video-1250000000。", a.COSBucket(), a)
			textRow(c, "cos-access", "Access Key", "填 SecretId。", a.COSAccessKey(), a)
			secretRow(c, "cos-secret", "填 SecretKey。", a.COSSecret(), a)
		case "compat":
			textRow(c, "compat-endpoint", "Endpoint", "完整的 https 地址。", a.CompatEndpoint(), a)
			textRow(c, "compat-region", "Region", "签名用的区域名。MinIO 通常填 us-east-1。", a.CompatRegion(), a)
			textRow(c, "compat-bucket", "Bucket", "", a.CompatBucket(), a)
			textRow(c, "compat-access", "Access Key", "", a.CompatAccessKey(), a)
			secretRow(c, "compat-secret", "", a.CompatSecret(), a)
			form.Separator(c, true)
			c.Key("compat-path")
			if form.SwitchRow(c, "路径样式", "MinIO 打开。B2、Wasabi、华为云、七牛保持关闭。", a.CompatPathStyle()) {
				_ = a.Save()
			}
		default:
			textRow(c, "r2-endpoint", "Endpoint", "填 Account ID，或完整的 https 地址。", a.R2Endpoint(), a)
			textRow(c, "r2-bucket", "Bucket", "", a.R2Bucket(), a)
			textRow(c, "r2-access", "Access Key", "", a.R2AccessKey(), a)
			secretRow(c, "r2-secret", "", a.R2Secret(), a)
		}
	})
	form.Note(c, "视频不会在下载完成时自动上传。到播放页点上传。各处的密钥分开保存，只存在本机设置文件里。", 0)
}

func storageRow(c *ui.Context, a App) {
	selected := a.Storage()
	// The drop-down shows the storage names; the setting keeps the id.
	label := storageLabel(*selected)
	labels := make([]string, len(storageKinds))
	for i, id := range storageKinds {
		labels[i] = storageLabel(id)
	}
	form.Row(c, "类型", "", func() {
		c.Key("storage-kind")
		if form.Select(c, &label, labels).Label("类型").Changed() {
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

// googleRow is the account card's row, as the prototype's: a gray avatar, the account over what it is for, and the button that signs in or out.
func googleRow(c *ui.Context, a App) {
	title, subtitle := "未登录", "登录后才能上传影片。"
	if email := a.GoogleEmail(); email != "" {
		title, subtitle = email, "通过 Google 登录"
	}
	ui.Row(c).Shrink(0).MinHeight(62).Padding(10, 14).Gap(12).AlignItems(ui.Center).Role(ui.RoleGroup).Label(title).Description(subtitle).Children(func() {
		ui.Box(c).Size(40, 40).Radius(20).Shrink(0).Background(form.Idle)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(form.Text).SingleLine()
			ui.Text(c, subtitle).FontSize(11).TextColor(form.Secondary).Wrap()
		})
		if a.GoogleEmail() != "" {
			c.Key("google-sign-out")
			if form.Button(c, "退出登录", false).Clicked() {
				a.SignOutGoogle()
			}
			return
		}
		label := "使用 Google 登录"
		if a.GoogleBusy() {
			label = "正在登录…"
		}
		c.Key("google-sign-in")
		btn := breeze.PrimaryButton(c, label).Height(28).FontSize(12).FontWeight(600).Disabled(a.GoogleBusy() || !a.GoogleConfigured())
		if a.GoogleConfigured() && !a.GoogleBusy() && btn.Clicked() {
			a.SignInGoogle()
		}
	})
}

// textRow edits one of the storage's fields in the prototype's column of fields.
func textRow(c *ui.Context, key, label, hint string, value *string, a App) {
	form.Separator(c, true)
	form.Row(c, label, hint, func() {
		c.Key(key)
		if field(breeze.TextInput(c, value).Label(label)).Font("monospace").FontSize(12).Changed() {
			_ = a.Save()
		}
	})
}

// secretRow edits a storage key's secret in a password field, which keeps it hidden on screen.
func secretRow(c *ui.Context, key, hint string, value *string, a App) {
	form.Separator(c, true)
	form.Row(c, "Secret", hint, func() {
		c.Key(key)
		if field(breeze.PasswordField(c, value).Label("Secret")).Changed() {
			_ = a.Save()
		}
	})
}

// field gives a text field in a row the prototype's room: 300 points, the width of the column the fields line up in, and 26 high.
func field(e ui.Element) ui.Element {
	return e.Width(300).Height(26)
}
