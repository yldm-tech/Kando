package account

import (
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

// fakeApp is the account and the storage as the app keeps them, plain fields, with the calls the page made.
type fakeApp struct {
	live.Rev
	email                                      string
	busy, configured                           bool
	failed                                     string
	storage                                    string
	r2Endpoint, r2Bucket, r2Access, r2Secret   string
	s3Region, s3Bucket, s3Access, s3Secret     string
	ossRegion, ossBucket, ossAccess, ossSecret string
	cosRegion, cosBucket, cosAccess, cosSecret string
	compatEndpoint, compatRegion, compatBucket string
	compatAccess, compatSecret                 string
	compatPath                                 bool
	saves                                      int
	calls                                      []string
}

func (f *fakeApp) GoogleEmail() string      { return f.email }
func (f *fakeApp) GoogleBusy() bool         { return f.busy }
func (f *fakeApp) GoogleErr() string        { return f.failed }
func (f *fakeApp) GoogleConfigured() bool   { return f.configured }
func (f *fakeApp) SignInGoogle()            { f.calls = append(f.calls, "sign in") }
func (f *fakeApp) SignOutGoogle()           { f.calls = append(f.calls, "sign out"); f.email = "" }
func (f *fakeApp) Storage() *string         { return &f.storage }
func (f *fakeApp) R2Endpoint() *string      { return &f.r2Endpoint }
func (f *fakeApp) R2Bucket() *string        { return &f.r2Bucket }
func (f *fakeApp) R2AccessKey() *string     { return &f.r2Access }
func (f *fakeApp) R2Secret() *string        { return &f.r2Secret }
func (f *fakeApp) S3Region() *string        { return &f.s3Region }
func (f *fakeApp) S3Bucket() *string        { return &f.s3Bucket }
func (f *fakeApp) S3AccessKey() *string     { return &f.s3Access }
func (f *fakeApp) S3Secret() *string        { return &f.s3Secret }
func (f *fakeApp) OSSRegion() *string       { return &f.ossRegion }
func (f *fakeApp) OSSBucket() *string       { return &f.ossBucket }
func (f *fakeApp) OSSAccessKey() *string    { return &f.ossAccess }
func (f *fakeApp) OSSSecret() *string       { return &f.ossSecret }
func (f *fakeApp) COSRegion() *string       { return &f.cosRegion }
func (f *fakeApp) COSBucket() *string       { return &f.cosBucket }
func (f *fakeApp) COSAccessKey() *string    { return &f.cosAccess }
func (f *fakeApp) COSSecret() *string       { return &f.cosSecret }
func (f *fakeApp) CompatEndpoint() *string  { return &f.compatEndpoint }
func (f *fakeApp) CompatRegion() *string    { return &f.compatRegion }
func (f *fakeApp) CompatBucket() *string    { return &f.compatBucket }
func (f *fakeApp) CompatAccessKey() *string { return &f.compatAccess }
func (f *fakeApp) CompatSecret() *string    { return &f.compatSecret }
func (f *fakeApp) CompatPathStyle() *bool   { return &f.compatPath }
func (f *fakeApp) Save() error              { f.saves++; return nil }

// StorageKind is the storage's kind as the app reads its setting: an id it does not know is R2.
func (f *fakeApp) StorageKind() string {
	for _, id := range storageKinds {
		if f.storage == id {
			return id
		}
	}
	return "r2"
}

// show draws the page in a settings window's room, on a Mac, in testlocale's language.
func show(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return core.Scroll{Style: core.Style{FlexGrow: 1, Padding: core.All(20)}, Children: []core.Node{View{App: a}}}
	}, nsgotest.Size(720, 900), nsgotest.Environment(env))
}

func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func whole(s string) string { return `text:"` + s + `"` }

// Signed out without a client ID, the row says so and 使用 Google 登录 is off; with one it signs in, says 正在登录 while it does, and once signed in shows the account with 退出登录.
func TestGoogleRow(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	for _, want := range []string{msgs.AccountSignedOut(), msgs.AccountSignedOutHint(), msgs.AccountNoClientNote()} {
		if !app.Exists(whole(want)) {
			t.Fatalf("no %q in %q", want, app.Texts())
		}
	}
	if app.Enabled("id:google-sign-in") {
		t.Fatal("sign-in is on without a client ID")
	}
	change(app, a, func() { a.configured = true })
	if !app.Exists(whole(msgs.AccountSignInNote())) {
		t.Fatalf("no %q in %q", msgs.AccountSignInNote(), app.Texts())
	}
	app.Press(whole(msgs.AccountSignIn()))
	change(app, a, func() { a.busy = true })
	if !app.Exists(whole(msgs.AccountSigningIn())) || app.Enabled("id:google-sign-in") {
		t.Fatalf("texts %q", app.Texts())
	}
	change(app, a, func() { a.busy, a.failed = false, msgs.AccountGoogleFailed() })
	if !app.Exists(whole(msgs.AccountGoogleFailed())) {
		t.Fatalf("no failure in %q", app.Texts())
	}
	change(app, a, func() { a.email, a.failed = "a@b.c", "" })
	if !app.Exists(whole("a@b.c")) || !app.Exists(whole(msgs.AccountSignedInWith())) || app.Find("id:google-row").Props.Label != "a@b.c" {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press(whole(msgs.AccountSignOut()))
	if want := []string{"sign in", "sign out"}; len(a.calls) != 2 || a.calls[0] != want[0] || a.calls[1] != want[1] || !app.Exists(whole(msgs.AccountSignIn())) {
		t.Fatalf("calls %q, texts %q", a.calls, app.Texts())
	}
}

// Each kind of storage shows its own fields with its hints; an edit is kept and saved as it is typed, and choosing another kind saves it.
func TestStorageFields(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	for _, id := range []string{"r2-endpoint", "r2-bucket", "r2-access", "r2-secret"} {
		if !app.Exists("id:" + id) {
			t.Fatalf("no %s on R2's form", id)
		}
	}
	if !app.Exists(whole(msgs.AccountR2EndpointHint())) || !app.Exists(whole("Cloudflare R2")) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Type("id:r2-bucket", "clips")
	if a.r2Bucket != "clips" || a.saves != len("clips") {
		t.Fatalf("bucket %q saved %d times", a.r2Bucket, a.saves)
	}
	if !app.Find("id:r2-secret").Input.Masked {
		t.Fatal("the secret shows on screen")
	}
	app.Press("id:storage-kind")
	app.Press("id:storage-kind-3")
	if a.storage != "cos" || !app.Exists(whole(msgs.AccountStoreCOS())) || !app.Exists(whole(msgs.AccountCOSBucketHint())) || app.Exists("id:r2-bucket") {
		t.Fatalf("storage %q, texts %q", a.storage, app.Texts())
	}
	change(app, a, func() { a.storage = "compat" })
	if !app.Exists(whole(msgs.AccountPathStyle())) || !app.Exists("id:compat-endpoint") {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press(whole(msgs.AccountPathStyleHint()))
	if !a.compatPath {
		t.Fatal("a press on the switch's words did not turn it")
	}
	app.Press(whole(msgs.AccountPathStyle()))
	if a.compatPath || len(app.QueryMounted("id:compat-path")) != 1 {
		t.Fatal("the switch's words did not turn path style off, or the switch is gone")
	}
}

// What the page reads follows the app's whole revision; a change that leaves the storage's kind the same builds nothing again, so a field keeps the keyboard and what is typed in it.
func TestPageStaysWhileTheAppChanges(t *testing.T) {
	a := &fakeApp{storage: "s3"}
	app := show(t, a)
	app.Type("id:s3-region", "eu-west-1")
	field := app.Find("id:s3-region").ID
	change(app, a, func() { a.busy = true })
	if v := app.Find("id:s3-region"); v.ID != field || !v.Focused || v.Text != "eu-west-1" {
		t.Fatalf("the field was built again: view %d (was %d), focused %v, holding %q", v.ID, field, v.Focused, v.Text)
	}
}

// The page follows the interface's language as it changes.
func TestPageFollowsTheLanguage(t *testing.T) {
	a := &fakeApp{storage: "compat"}
	app := show(t, a)
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true, Locale: "en"})
	for _, want := range []string{"Account", "Not Signed In", "Object Storage", "S3 Compatible", "Path Style", "The full https address."} {
		if !app.Exists(whole(want)) {
			t.Errorf("no %q in %q", want, app.Texts())
		}
	}
}
