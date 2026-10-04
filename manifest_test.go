package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseManifest(t *testing.T) {
	for file, want := range map[string][]string{
		"one-type.json":  {"hue"},
		"two-types.json": {"sonos", "sonos-sub"},
	} {
		types, err := ParseManifest(readFile(t, filepath.Join("testdata/manifest/valid", file)))
		if err != nil {
			t.Errorf("%s: %v", file, err)
		} else if got := slices.Sorted(maps.Keys(types)); !slices.Equal(got, want) {
			t.Errorf("%s: types %v, want %v", file, got, want)
		}
	}
}

func TestParseManifestRefuses(t *testing.T) {
	files, err := filepath.Glob("testdata/manifest/invalid/*.json")
	if err != nil || len(files) == 0 {
		t.Fatal("no invalid manifests", err)
	}
	for _, file := range files {
		if types, err := ParseManifest(readFile(t, file)); err == nil {
			t.Errorf("%s: accepted: %v", file, types)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
