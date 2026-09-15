package registry

import (
	"strings"
	"testing"
)

const goodManifest = `{
  "name": "hello",
  "display_name": "Hello",
  "description": "Says hi.",
  "author": "Jason Ernst",
  "license": "Apache-2.0",
  "entry": "plugin.go",
  "min_goblog_version": "0.2.6",
  "homepage": "https://example.test"
}`

func TestParseManifest_Good(t *testing.T) {
	m, err := ParseManifest([]byte(goodManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "hello" || m.DisplayName != "Hello" || m.License != "Apache-2.0" || m.Entry != "plugin.go" || m.MinGoblogVersion != "0.2.6" || m.Homepage != "https://example.test" {
		t.Errorf("unexpected manifest: %+v", m)
	}
}

func TestParseManifest_EntryDefaultsToPluginGo(t *testing.T) {
	m, err := ParseManifest([]byte(strings.Replace(goodManifest, `"entry": "plugin.go",`, "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if m.Entry != "plugin.go" {
		t.Errorf("entry should default to plugin.go, got %q", m.Entry)
	}
}

func TestParseManifest_Errors(t *testing.T) {
	cases := map[string]string{
		"not json":            `{`,
		"missing name":        strings.Replace(goodManifest, `"name": "hello",`, "", 1),
		"bad name":            strings.Replace(goodManifest, `"name": "hello"`, `"name": "Hello_World"`, 1),
		"missing display":     strings.Replace(goodManifest, `"display_name": "Hello",`, "", 1),
		"missing description": strings.Replace(goodManifest, `"description": "Says hi.",`, "", 1),
		"missing author":      strings.Replace(goodManifest, `"author": "Jason Ernst",`, "", 1),
		"unknown license":     strings.Replace(goodManifest, `"Apache-2.0"`, `"MyLicense"`, 1),
		"entry not go":        strings.Replace(goodManifest, `"plugin.go"`, `"plugin.txt"`, 1),
		"entry with slash":    strings.Replace(goodManifest, `"plugin.go"`, `"src/plugin.go"`, 1),
		"min version with v":  strings.Replace(goodManifest, `"0.2.6"`, `"v0.2.6"`, 1),
		"min version junk":    strings.Replace(goodManifest, `"0.2.6"`, `"latest"`, 1),
		"missing min version": strings.Replace(goodManifest, `"min_goblog_version": "0.2.6",`, "", 1),
	}
	for name, src := range cases {
		if _, err := ParseManifest([]byte(src)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
