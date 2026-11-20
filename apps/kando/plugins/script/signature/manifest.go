package signature

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"kando/apps/kando/plugins"
)

// SignedKeys are the keys of a script plugin's manifest the signature must cover, as kando-plugin.json sits outside it (docs/plugin-system.md 4.3): what the plugin may reach and adds. A package holds them in its signed metadata (pkg.Contents.Extra, SignedManifest), or its code returns them from kandoManifest.
var SignedKeys = []string{"capabilities", "optional", "contributes", "credentials", "settings", "events"}

// ManifestEntrySince is the oldest Kando that does not refuse a package holding its signed manifest (pkg.Contents.Extra): 0.37.0 and older are built with NSGo ec272782be28, which does not know the entry, lists it in Info.Ignored, and Verify refuses the package for it (checkSigned); every release after 0.37.0 is built from a main that pins NSGo 729f238f5678 or newer, which knows it. It is not the oldest Kando that compares the entry in place of kandoManifest: a release built before Verify compared Info.Extra takes the package but still runs its code to ask it (script.Probe), so a package that signs its manifest keeps exporting kandoManifest until its manifest's oldest Kando is one that compares the entry, which is known only once that release ships.
const ManifestEntrySince = "0.38.0"

// SignsManifest reports whether the package of the script plugin m may hold its signed manifest (SignedManifest): only when no Kando m runs on refuses it, its oldest (Manifest.Kando) ManifestEntrySince or newer. Whether its code may leave out kandoManifest is another matter (ManifestEntrySince).
func SignsManifest(m plugins.Manifest) bool {
	oldest, ok := strings.CutPrefix(m.Kando, ">=")
	return ok && plugins.Manifest{Kando: ">=" + ManifestEntrySince}.RunsOn(oldest)
}

// SignedManifest is what a package of the script plugin m signs of its manifest (pkg.Contents.Extra): a JSON object of m's SignedKeys, as JSON reads them and what is empty left out, which Verify compares with the manifest (CheckManifest).
func SignedManifest(m plugins.Manifest) ([]byte, error) {
	all, err := manifestKeys(m)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, k := range SignedKeys {
		if v := normalize(all[k]); v != nil {
			out[k] = v
		}
	}
	return json.Marshal(out)
}

// CheckManifest checks that signed, a JSON object of what a package signed of its manifest (SignedManifest) or what its code says of it (kandoManifest), holds the SignedKeys of m, alike once both are read as JSON reads them and what is empty is left out, and no other key: plugins.ErrTampered otherwise.
func CheckManifest(signed []byte, m plugins.Manifest) error {
	var said map[string]any
	if err := json.Unmarshal(signed, &said); err != nil || said == nil {
		return fmt.Errorf("%w: what it says of itself is no object", plugins.ErrTampered)
	}
	all, err := manifestKeys(m)
	if err != nil {
		return err
	}
	for k := range said {
		if !slices.Contains(SignedKeys, k) && normalize(said[k]) != nil {
			return fmt.Errorf("%w: it says %s of itself", plugins.ErrTampered, k)
		}
	}
	for _, k := range SignedKeys {
		if !reflect.DeepEqual(normalize(said[k]), normalize(all[k])) {
			return fmt.Errorf("%w: the %s it says of itself is not the manifest's", plugins.ErrTampered, k)
		}
	}
	return nil
}

// manifestKeys is m as JSON reads it, an object of its keys.
func manifestKeys(m plugins.Manifest) (map[string]any, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var all map[string]any
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	return all, nil
}

// normalize is v as JSON decodes it, with empty arrays, empty objects and null left out of what holds them, so a key left out and one written empty are alike.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if n := normalize(e); n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	}
	return v
}
