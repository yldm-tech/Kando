package account

import (
	"strings"

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

// Show draws the 账号与上传 category: the Google account uploads are filed under, and the object storage they go to.
func Show(c *ui.Context, t *ui.Theme, a App) {
	breeze.PreferencesGroup(c, "Google", "登录只用来标明是谁上传的。刷新令牌保存在本机设置文件里。", nil, func() {
		googleRow(c, a)
	})
	if a.GoogleErr() != "" {
		breeze.Text(c, a.GoogleErr(), breeze.TextCaption).TextColor(t.Danger).Padding(0, 10).Wrap()
	}
	breeze.PreferencesGroup(c, "对象存储", "视频不会在下载完成时自动上传。到播放页点上传。各处的密钥分开保存，只存在本机设置文件里。", nil, func() {
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
			c.Key("compat-path")
			if breeze.SwitchRow(c, "路径样式", "MinIO 打开。B2、Wasabi、华为云、七牛保持关闭。", a.CompatPathStyle()).Changed() {
				_ = a.Save()
			}
		default:
			textRow(c, "r2-endpoint", "Endpoint", "填 Account ID，或完整的 https 地址。", a.R2Endpoint(), a)
			textRow(c, "r2-bucket", "Bucket", "", a.R2Bucket(), a)
			textRow(c, "r2-access", "Access Key", "", a.R2AccessKey(), a)
			secretRow(c, "r2-secret", "", a.R2Secret(), a)
		}
	})

}

func storageRow(c *ui.Context, a App) {
	selected := a.Storage()
	// The drop-down shows the storage names; the setting keeps the id.
	label := storageLabel(*selected)
	labels := make([]string, len(storageKinds))
	for i, id := range storageKinds {
		labels[i] = storageLabel(id)
	}
	c.Key("storage-kind")
	if breeze.ComboRow(c, "位置", "", &label, labels).Changed() {
		for _, id := range storageKinds {
			if storageLabel(id) == label {
				*selected = id
			}
		}
		_ = a.Save()
	}
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

func googleRow(c *ui.Context, a App) {
	title, subtitle := "未登录", "登录后才能上传影片。"
	if email := a.GoogleEmail(); email != "" {
		title, subtitle = email, "通过 Google 登录"
	}
	if !a.GoogleConfigured() && a.GoogleEmail() == "" {
		subtitle = "登录需要环境变量 LINGOCUT_GOOGLE_CLIENT_ID。Client ID 不写入设置文件。"
	}
	breeze.ActionRow(c, title, subtitle, func() {
		if a.GoogleEmail() != "" {
			c.Key("google-sign-out")
			if breeze.Button(c, "退出登录").Clicked() {
				a.SignOutGoogle()
			}
			return
		}
		label := "使用 Google 登录"
		if a.GoogleBusy() {
			label = "正在登录…"
		}
		c.Key("google-sign-in")
		btn := breeze.PrimaryButton(c, label).Disabled(a.GoogleBusy() || !a.GoogleConfigured())
		if a.GoogleConfigured() && !a.GoogleBusy() && btn.Clicked() {
			a.SignInGoogle()
		}
	})
}

func textRow(c *ui.Context, key, label, hint string, value *string, a App) {
	breeze.ActionRow(c, label, hint, func() {
		c.Key(key)
		if field(breeze.TextInput(c, value).Label(label)).Changed() {
			_ = a.Save()
		}
	})
}

// secretRow edits a storage key's secret in a password field, which keeps it hidden on screen.
func secretRow(c *ui.Context, key, hint string, value *string, a App) {
	breeze.ActionRow(c, "Secret", hint, func() {
		c.Key(key)
		if field(breeze.PasswordField(c, value).Label("Secret")).Changed() {
			_ = a.Save()
		}
	})
}

// field gives a text field in a row its room: wide enough for an endpoint, narrow enough to leave the title its line.
func field(e ui.Element) ui.Element {
	return e.Width(280)
}
