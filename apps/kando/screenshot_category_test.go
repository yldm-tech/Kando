package main

import (
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/category"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// screenshotCategories sorts the prototype's done films into three categories, 纪录片, 课程 and 听力, leaving 会议录屏 in none, and returns their IDs by name. The categories are kept in the library's folder, which every scene shares, so they go when the scene does.
func screenshotCategories(t *testing.T, f *screenshotFixture, a *app) map[string]string {
	t.Helper()
	t.Cleanup(func() { os.Remove(filepath.Join(f.lib, category.FileName)) })
	sorting := map[string][]string{
		"纪录片": {"j1", "j8", "j9", "j10"},
		"课程":  {"j3", "j11"},
		"听力":  {"j15", "j1"},
	}
	ids := map[string]string{}
	for _, name := range []string{"纪录片", "课程", "听力"} {
		var paths []string
		for _, job := range sorting[name] {
			paths = append(paths, f.job(t, job).VideoPath)
		}
		a.AskNewCategory(paths...)
		a.CategoryNaming().Name = name
		a.SubmitCategoryName()
		for _, c := range a.Categories() {
			if c.Name == name {
				ids[name] = c.ID
			}
		}
		if ids[name] == "" {
			t.Fatalf("the category %s was not made", name)
		}
	}
	return ids
}

// TestScreenshotsCategory renders the library's categories (package category):
//   - category-sidebar: the 分类 section under 观看进度, three categories and 未分类 with their counts, and 纪录片 chosen, the grid showing its films
//   - category-empty: a library whose films are in no category yet, the section's + and the line saying what categories are for
//   - category-name: the + pressed, 动漫 typed in the dialog that names a new category
//   - category-name-taken: 课程 typed, a name a category has, and the line under the field saying so
//   - category-delete: 删除分类… for 听力, its confirmation
func TestScreenshotsCategory(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "category-sidebar", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		ids := screenshotCategories(t, f, a)
		refresh(tt, a)
		tt.Press("id:category-" + ids["纪录片"])
		tt.HoverOut()
	})
	f.shoot(t, "category-empty", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		refresh(tt, a)
		if !tt.Exists("id:category-empty") {
			t.Fatal("the section does not say what categories are for")
		}
	})
	f.shoot(t, "category-name", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		refresh(tt, a)
		tt.Press("id:category-add")
		refresh(tt, a)
		typeFocused(t, tt, "动漫")
	})
	f.shoot(t, "category-name-taken", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		screenshotCategories(t, f, a)
		refresh(tt, a)
		tt.Press("id:category-add")
		refresh(tt, a)
		typeFocused(t, tt, "课程")
		tt.Submit("id:category-name-field")
		refresh(tt, a)
		if !tt.Exists("id:category-name-problem") {
			t.Fatal("a name taken is not refused")
		}
	})
	f.shoot(t, "category-delete", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		ids := screenshotCategories(t, f, a)
		a.AskDeleteCategory(ids["听力"])
		refresh(tt, a)
		if _, ok := a.CategoryDeleting(); !ok || !tt.Exists("id:category-delete") {
			t.Fatal("the confirmation does not show")
		}
	})
}
