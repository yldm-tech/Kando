package account

import (
	"strings"

	"kando/apps/kando/locales"
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
	form.Label(c, locales.AccountSectionAccount(), 6, nil)
	form.Card(c, func() { googleRow(c, a) })
	if a.GoogleErr() != "" {
		ui.Text(c, a.GoogleErr()).FontSize(11).TextColor(t.Danger).Wrap().Padding(0, 12)
	}
	note := locales.AccountSignInNote()
	if !a.GoogleConfigured() && a.GoogleEmail() == "" {
		note = locales.AccountNoClientNote()
	}
	form.Note(c, note, 10)
	form.Label(c, locales.AccountSectionStorage(), 6, nil)
	form.Card(c, func() {
		storageRow(c, a)
		switch a.StorageKind() {
		case "s3":
			textRow(c, "s3-region", "Region", locales.AccountS3RegionHint(), a.S3Region(), a)
			textRow(c, "s3-bucket", "Bucket", "", a.S3Bucket(), a)
			textRow(c, "s3-access", "Access Key", "", a.S3AccessKey(), a)
			secretRow(c, "s3-secret", "", a.S3Secret(), a)
		case "oss":
			textRow(c, "oss-region", "Region", locales.AccountOSSRegionHint(), a.OSSRegion(), a)
			textRow(c, "oss-bucket", "Bucket", "", a.OSSBucket(), a)
			textRow(c, "oss-access", "Access Key", "", a.OSSAccessKey(), a)
			secretRow(c, "oss-secret", "", a.OSSSecret(), a)
		case "cos":
			textRow(c, "cos-region", "Region", locales.AccountCOSRegionHint(), a.COSRegion(), a)
			textRow(c, "cos-bucket", "Bucket", locales.AccountCOSBucketHint(), a.COSBucket(), a)
			textRow(c, "cos-access", "Access Key", locales.AccountCOSAccessHint(), a.COSAccessKey(), a)
			secretRow(c, "cos-secret", locales.AccountCOSSecretHint(), a.COSSecret(), a)
		case "compat":
			textRow(c, "compat-endpoint", "Endpoint", locales.AccountCompatEndpointHint(), a.CompatEndpoint(), a)
			textRow(c, "compat-region", "Region", locales.AccountCompatRegionHint(), a.CompatRegion(), a)
			textRow(c, "compat-bucket", "Bucket", "", a.CompatBucket(), a)
			textRow(c, "compat-access", "Access Key", "", a.CompatAccessKey(), a)
			secretRow(c, "compat-secret", "", a.CompatSecret(), a)
			form.Separator(c, true)
			c.Key("compat-path")
			if form.SwitchRow(c, locales.AccountPathStyle(), locales.AccountPathStyleHint(), a.CompatPathStyle()) {
				_ = a.Save()
			}
		default:
			textRow(c, "r2-endpoint", "Endpoint", locales.AccountR2EndpointHint(), a.R2Endpoint(), a)
			textRow(c, "r2-bucket", "Bucket", "", a.R2Bucket(), a)
			textRow(c, "r2-access", "Access Key", "", a.R2AccessKey(), a)
			secretRow(c, "r2-secret", "", a.R2Secret(), a)
		}
	})
	form.Note(c, locales.AccountUploadNote(), 0)
}

func storageRow(c *ui.Context, a App) {
	selected := a.Storage()
	// The drop-down shows the storage names; the setting keeps the id.
	label := storageLabel(*selected)
	labels := make([]string, len(storageKinds))
	for i, id := range storageKinds {
		labels[i] = storageLabel(id)
	}
	form.Row(c, locales.AccountStorageKind(), "", func() {
		c.Key("storage-kind")
		if form.Select(c, &label, labels).Label(locales.AccountStorageKind()).Changed() {
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

// storageLabel is the storage's name as the drop-down shows it, in the interface's language; the setting keeps the id.
func storageLabel(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "s3":
		return "Amazon S3"
	case "oss":
		return locales.AccountStoreOSS()
	case "cos":
		return locales.AccountStoreCOS()
	case "compat":
		return locales.AccountStoreCompat()
	default:
		return "Cloudflare R2"
	}
}

// googleRow is the account card's row, as the prototype's: a gray avatar, the account over what it is for, and the button that signs in or out.
func googleRow(c *ui.Context, a App) {
	title, subtitle := locales.AccountSignedOut(), locales.AccountSignedOutHint()
	if email := a.GoogleEmail(); email != "" {
		title, subtitle = email, locales.AccountSignedInWith()
	}
	ui.Row(c).Shrink(0).MinHeight(62).Padding(10, 14).Gap(12).AlignItems(ui.Center).Role(ui.RoleGroup).Label(title).Description(subtitle).Children(func() {
		ui.Box(c).Size(40, 40).Radius(20).Shrink(0).Background(form.Idle)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(form.Text).SingleLine()
			ui.Text(c, subtitle).FontSize(11).TextColor(form.Secondary).Wrap()
		})
		if a.GoogleEmail() != "" {
			c.Key("google-sign-out")
			if form.Button(c, locales.AccountSignOut(), false).Clicked() {
				a.SignOutGoogle()
			}
			return
		}
		label := locales.AccountSignIn()
		if a.GoogleBusy() {
			label = locales.AccountSigningIn()
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
