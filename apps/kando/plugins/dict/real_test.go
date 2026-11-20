package dict

import (
	"archive/zip"
	"bufio"
	"cmp"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/plugins"
)

// TestRealDictionary measures a real dictionary file, as JMdict, Jitendex or CC-CEDICT, packed as Kando packs it (docs/plugin-system.md, section 4.1): the entries and bytes of the pack against the limits plugins.Validate keeps, how long Validate and building the index (Open) take, the process's peak memory while the index is built, and how long a lookup takes. It runs only when KANDO_DICT_BENCH names the file (a Yomitan zip, or CC-CEDICT text), so CI never downloads a dictionary; KANDO_DICT_BENCH_LANG is its language, ja or zh (ja by default). Run it once a process, as the peak memory is the process's: KANDO_DICT_BENCH=JMdict_english.zip go test ./plugins/dict -run TestRealDictionary -v. It needs Linux for the memory (/proc/self/status).
func TestRealDictionary(t *testing.T) {
	src := os.Getenv("KANDO_DICT_BENCH")
	if src == "" {
		t.Skip("KANDO_DICT_BENCH names no dictionary file")
	}
	lang := cmp.Or(os.Getenv("KANDO_DICT_BENCH_LANG"), "ja")
	format := plugins.FormatCCCEDICT
	if strings.EqualFold(filepath.Ext(src), ".zip") {
		format = plugins.FormatYomitan
	}
	dir := t.TempDir()
	name := filepath.Base(src)
	if err := copyFile(filepath.Join(dir, name), src); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]any{
		"schema": 1, "id": "bench.dict", "kind": "dictionary", "version": "1", "license": "CC-BY-SA-4.0",
		"name":       map[string]string{"en": "Bench"},
		"dictionary": map[string]any{"format": format, "files": []string{name}, "from": []string{lang}, "to": []string{"en"}},
	})
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestName), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("file %s: %.1f MiB (a dictionary's zip may be 200 MiB)", name, mib(info.Size()))
	if format == plugins.FormatYomitan {
		zipShape(t, src)
	}

	start := time.Now()
	_, verr := plugins.Validate(dir)
	t.Logf("plugins.Validate: %v (nil is ok) in %v", verr, time.Since(start).Round(time.Millisecond))

	runtime.GC()
	debug.FreeOSMemory()
	before := memory(t)
	// Writing 5 to clear_refs sets the peak (VmHWM) back to what is resident now, so the peak below is the build's.
	os.WriteFile("/proc/self/clear_refs", []byte("5"), 0)
	p := plugins.Pack{Manifest: plugins.Manifest{ID: "bench.dict", Kind: plugins.KindDictionary, Dictionary: &plugins.Dictionary{Format: format, Files: []string{name}}}, Dir: dir, Hash: "0123456789abcdef"}
	cache := filepath.Join(t.TempDir(), ".index")
	start = time.Now()
	d, err := Open(p, cache)
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	peak := memory(t)
	idx, _ := os.Stat(filepath.Join(cache, IndexName(p.ID, p.Hash)))
	t.Logf("Open (build the index): %v, %d keys, index %.1f MiB", took.Round(time.Millisecond), d.count, mib(idx.Size()))
	t.Logf("memory before: VmRSS %.1f MiB; during the build: VmHWM %.1f MiB (+%.1f MiB)", mib(before.rss), mib(peak.hwm), mib(peak.hwm-before.rss))

	d.Close()
	start = time.Now()
	d, err = Open(p, cache)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Open (index already built): %v", time.Since(start).Round(time.Microsecond))

	words := map[string][][3]string{
		"ja": {{"食べた", "食べる", "タベタ"}, {"行った", "行く", "イッタ"}, {"猫", "", "ネコ"}, {"日本語", "", ""}, {"ありがとう", "", ""}, {"学校", "", ""}, {"先生", "", ""}, {"見る", "", "ミル"}, {"生", "", "ナマ"}, {"ぱぴぷぺぽー", "", ""}},
		"zh": {{"中国", "", ""}, {"学习", "", ""}, {"學習", "", ""}, {"你好", "", ""}, {"电脑", "", ""}, {"電腦", "", ""}, {"喜欢", "", ""}, {"行", "", ""}, {"了", "", ""}, {"啊啊啊啊", "", ""}},
	}[lang]
	for _, w := range words[:3] {
		e, ok, err := d.LookUp(w[0], w[1], w[2])
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %v %q %q %q", w[0], ok, e.Word, e.Reading, clip(strings.ReplaceAll(e.Meaning, "\n", " | "), 300))
	}
	var times []time.Duration
	hits := 0
	for range 1000 {
		for _, w := range words {
			start := time.Now()
			_, ok, err := d.LookUp(w[0], w[1], w[2])
			times = append(times, time.Since(start))
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				hits++
			}
		}
	}
	slices.Sort(times)
	var sum time.Duration
	for _, d := range times {
		sum += d
	}
	t.Logf("LookUp x%d (%d found, page cache warm): mean %v, p50 %v, p99 %v, max %v", len(times), hits, sum/time.Duration(len(times)), times[len(times)/2], times[len(times)*99/100], times[len(times)-1])
}

// zipShape logs what the Yomitan zip at name holds against a dictionary pack's limits: its entries, of which a pack may have 64 with its own files (the manifest and this zip), what they unpack to (1 GiB), and its central directory, of which openZip reads at most 1 MiB.
func zipShape(t *testing.T, name string) {
	zr, err := zip.OpenReader(name)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	kinds := map[string]int{}
	var unpacked uint64
	for _, zf := range zr.File {
		base := filepath.Base(zf.Name)
		switch {
		case strings.HasPrefix(base, "term_bank_"):
			kinds["term_bank"]++
		case strings.HasPrefix(base, "term_meta_bank_"):
			kinds["term_meta_bank"]++
		case strings.HasPrefix(base, "kanji_bank_"):
			kinds["kanji_bank"]++
		case strings.HasSuffix(base, ".json"):
			kinds["other json"]++
		case zf.Mode().IsDir():
			kinds["folder"]++
		default:
			kinds["other"]++
		}
		unpacked += zf.UncompressedSize64
	}
	t.Logf("zip: %d entries (%d with the manifest and the zip; the limit is 64) %v; unpack to %.1f MiB (the limit is 1024); central directory %.1f KiB (openZip reads 1024)", len(zr.File), len(zr.File)+2, kinds, mib(int64(unpacked)), float64(directorySize(t, name))/1024)
}

// directorySize is the size of the central directory of the zip at name, as its end of central directory record says.
func directorySize(t *testing.T, name string) int64 {
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, _ := f.Stat()
	n := min(info.Size(), 65536+22)
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, info.Size()-n); err != nil {
		t.Fatal(err)
	}
	for i := len(buf) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(buf[i:]) == 0x06054b50 {
			return int64(binary.LittleEndian.Uint32(buf[i+12:]))
		}
	}
	t.Fatal("no end of central directory")
	return 0
}

type usage struct{ rss, hwm int64 }

// memory is the process's resident memory now (VmRSS) and its peak (VmHWM), from /proc/self/status.
func memory(t *testing.T) usage {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		t.Skipf("no /proc/self/status: %v", err)
	}
	defer f.Close()
	var u usage
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var kb int64
		if n, _ := fmt.Sscanf(sc.Text(), "VmRSS: %d kB", &kb); n == 1 {
			u.rss = kb << 10
		}
		if n, _ := fmt.Sscanf(sc.Text(), "VmHWM: %d kB", &kb); n == 1 {
			u.hwm = kb << 10
		}
	}
	return u
}

func mib(n int64) float64 { return float64(n) / (1 << 20) }

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
