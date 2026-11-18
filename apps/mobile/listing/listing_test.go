package listing

import (
	"encoding/json"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"kando/apps/mobile/locales"
)

// appStoreLocales are the App Store's names of the app's languages, by the catalog's.
var appStoreLocales = map[string]string{"en-US": "en", "zh-Hans": "zh-Hans", "zh-Hant": "zh-Hant", "ja": "ja", "ko": "ko"}

// playLocales are Google Play's names of the app's languages, by the catalog's.
var playLocales = map[string]string{"en-US": "en", "zh-CN": "zh-Hans", "zh-TW": "zh-Hant", "ja-JP": "ja", "ko-KR": "ko"}

// limit is a field of a listing: its file and the most characters the store takes in it.
type limit struct {
	file string
	max  int
}

// The App Store Connect and Play Console limits, in characters.
var (
	appStoreFields = []limit{{"name.txt", 30}, {"subtitle.txt", 30}, {"promotional_text.txt", 170}, {"description.txt", 4000}, {"keywords.txt", 100}, {"support_url.txt", 0}, {"marketing_url.txt", 0}}
	playFields     = []limit{{"title.txt", 30}, {"short_description.txt", 80}, {"full_description.txt", 4000}}
)

// read returns a listing file's text without the line break that ends it.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

// dirs lists the directories in dir.
func dirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func checkFields(t *testing.T, dir string, fields []limit) {
	t.Helper()
	for _, f := range fields {
		text := read(t, filepath.Join(dir, f.file))
		switch {
		case text == "":
			t.Errorf("%s/%s is empty", dir, f.file)
		case strings.HasSuffix(f.file, "_url.txt"):
			if !strings.HasPrefix(text, "https://") || strings.ContainsAny(text, " \n") {
				t.Errorf("%s/%s is %q, not one https URL", dir, f.file, text)
			}
		case utf8.RuneCountInString(text) > f.max:
			t.Errorf("%s/%s has %d characters; the store takes %d", dir, f.file, utf8.RuneCountInString(text), f.max)
		}
	}
}

// Every language the app speaks has an App Store listing within App Store Connect's limits, named as the app is on the home screen.
func TestAppStoreListings(t *testing.T) {
	got := slices.DeleteFunc(dirs(t, "appstore"), func(d string) bool { return d == "review_information" })
	want := mapKeys(appStoreLocales)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("appstore has the languages %v, want %v", got, want)
	}
	for dir, lang := range appStoreLocales {
		path := filepath.Join("appstore", dir)
		checkFields(t, path, appStoreFields)
		if name, app := read(t, filepath.Join(path, "name.txt")), locales.In(lang).AppName(); !strings.HasPrefix(name, app) {
			t.Errorf("%s/name.txt is %q; begin it with the app's name, %q, as the home screen shows it", path, name, app)
		}
		// App Store Connect takes keywords separated by commas, where a space counts against the limit and a repeated word adds nothing.
		words := strings.Split(read(t, filepath.Join(path, "keywords.txt")), ",")
		seen := map[string]bool{}
		for _, w := range words {
			if w == "" || w != strings.TrimSpace(w) || seen[w] {
				t.Errorf("%s/keywords.txt has %q: keep one of each keyword, with no spaces around the commas", path, w)
			}
			seen[w] = true
		}
	}
	if notes := read(t, filepath.Join("appstore", "review_information", "notes.txt")); utf8.RuneCountInString(notes) > 4000 {
		t.Errorf("the notes for App Review have %d characters; App Store Connect takes 4000", utf8.RuneCountInString(notes))
	}
	if read(t, filepath.Join("appstore", "primary_category.txt")) != "EDUCATION" {
		t.Error("appstore/primary_category.txt is not EDUCATION")
	}
}

// Every language the app speaks has a Google Play listing within Play Console's limits, named as the launcher shows the app.
func TestPlayListings(t *testing.T) {
	got := dirs(t, "play")
	want := mapKeys(playLocales)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("play has the languages %v, want %v", got, want)
	}
	for dir, lang := range playLocales {
		path := filepath.Join("play", dir)
		checkFields(t, path, playFields)
		if title, app := read(t, filepath.Join(path, "title.txt")), locales.In(lang).AppName(); !strings.HasPrefix(title, app) {
			t.Errorf("%s/title.txt is %q; begin it with the app's name, %q", path, title, app)
		}
	}
}

// offSite are the words of the copy that would describe downloading from video sites, which the phone app does not do and the store copy leaves out: the computer does it, and app review takes such a description as the app's (yldm-tech/Kando#227).
var offSite = []string{
	"youtube", "bilibili", "yt-dlp", "tiktok", "vimeo", "niconico", "video site", "any site", "download video", "downloads video", "downloading video",
	"视频网站", "視頻網站", "影片網站", "下载视频", "下載影片", "下載視頻", "油管", "b站", "哔哩哔哩", "嗶哩嗶哩",
	"動画サイト", "動画をダウンロード", "ユーチューブ",
	"동영상 사이트", "동영상 다운로드", "동영상을 다운로드", "유튜브",
}

// No listing, nor the notes for App Review, describes downloading from video sites.
func TestListingsLeaveOutVideoSites(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".txt" {
			return err
		}
		text := strings.ToLower(read(t, path))
		for _, w := range offSite {
			if strings.Contains(text, w) {
				t.Errorf("%s says %q, which describes downloading from video sites", path, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The App Store's App Privacy answer is Data Not Collected, as privacy.md explains, in the form fastlane uploads.
func TestAppPrivacyDetails(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("appstore", "app_privacy_details.json"))
	if err != nil {
		t.Fatal(err)
	}
	var details []struct {
		Category        string   `json:"category"`
		DataProtections []string `json:"data_protections"`
	}
	if err := json.Unmarshal(data, &details); err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0].Category != "" || !slices.Equal(details[0].DataProtections, []string{"DATA_NOT_COLLECTED"}) {
		t.Errorf("app_privacy_details.json is %s; it should answer Data Not Collected alone", data)
	}
}

// The phone app's icon (nsgo.yaml's icon.png) is a 1024-pixel square with no transparency: App Store Connect refuses an iOS app icon that is transparent or has an alpha channel, and the systems round the square themselves.
func TestIconIsAnOpaqueSquare(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "assets", "Icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("the icon is %dx%d, want 1024x1024", b.Dx(), b.Dy())
	}
	// nsgo build scales the icon and writes it again, as RGB when every pixel is opaque.
	if o, ok := img.(interface{ Opaque() bool }); !ok || !o.Opaque() {
		t.Error("the icon has transparent pixels; fill them, as the App Store takes no transparency in an iOS icon")
	}
}

func mapKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
