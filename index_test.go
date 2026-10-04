package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fixture(t *testing.T) Index {
	t.Helper()
	ix, err := ParseIndex(readFile(t, "testdata/index.json"))
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func TestIndexRoundTrips(t *testing.T) {
	data := readFile(t, "testdata/index.json")
	if got := fixture(t).Encode(); !bytes.Equal(got, data) {
		t.Errorf("encoded:\n%s", got)
	}
	empty := Index{}.Encode()
	if string(empty) != "{\n  \"modules\": [],\n  \"rejected\": []\n}\n" {
		t.Errorf("empty index: %s", empty)
	}
	if _, err := ParseIndex(empty); err != nil {
		t.Error(err)
	}
}

func TestParseIndexRefuses(t *testing.T) {
	data := readFile(t, "testdata/index.json")
	for name, data := range map[string][]byte{
		"not canonical": bytes.ReplaceAll(data, []byte("  "), []byte("\t")),
		"unknown field": bytes.Replace(data, []byte(`"compatible": true`), []byte(`"compatible": true, "stars": 3`), 1),
		"trailing data": append(slices.Clone(data), "{}"...),
	} {
		if _, err := ParseIndex(data); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestIndexCheck(t *testing.T) {
	hue := func(ix *Index) *Module { return &ix.Modules[1] }
	for name, change := range map[string]func(*Index){
		"module path":       func(ix *Index) { hue(ix).Module = "github.com/someone/oiko-hue;rm -rf" },
		"repo not GitHub":   func(ix *Index) { hue(ix).Repo = "https://evil.example/someone/oiko-hue" },
		"repo path":         func(ix *Index) { hue(ix).Repo = "https://github.com/someone/../oiko-hue" },
		"invalid type":      func(ix *Index) { hue(ix).Types["hue"] = Type{"Hue", json.RawMessage(`{"type": "lifx"}`)} },
		"no types":          func(ix *Index) { hue(ix).Types = nil },
		"no versions":       func(ix *Index) { hue(ix).Versions = nil },
		"pre-release":       func(ix *Index) { hue(ix).Versions[0].Version = "v1.0.0-rc.1" },
		"not canonical":     func(ix *Index) { hue(ix).Versions[0].Version = "v1.0" },
		"min Oiko":          func(ix *Index) { hue(ix).Versions[0].MinOiko = "latest" },
		"unsorted versions": func(ix *Index) { v := hue(ix).Versions; v[0], v[1] = v[1], v[0] },
		"latest not last":   func(ix *Index) { hue(ix).Latest = "v1.1.0" },
		"oiko":              func(ix *Index) { hue(ix).Oiko = "" },
		"compatible, error": func(ix *Index) { hue(ix).Error = "boom" },
		"no error":          func(ix *Index) { ix.Modules[0].Error = "" },
		"listed twice":      func(ix *Index) { ix.Modules = append(ix.Modules, *hue(ix)) },
		"no reason":         func(ix *Index) { ix.Rejected[0].Reason = "" },
		"rejected repo":     func(ix *Index) { ix.Rejected[0].Repo = "someone/oiko-lifx" },
	} {
		ix := fixture(t)
		change(&ix)
		if err := ix.Check(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAddedTypes(t *testing.T) {
	version := readFile(t, "testdata/version.txt")
	for mod, want := range map[string][]string{
		"github.com/someone/oiko-hue": {"hue", "hue-sensor"},
		"example.com/oiko-sonos":      {"sonos"},
		"github.com/llehouerou/oiko":  {},
		"example.com/oiko-lifx":       {},
	} {
		if got := addedTypes(version, mod); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", mod, got, want)
		}
	}
}

func TestMinOiko(t *testing.T) {
	if got, err := minOiko("testdata/oiko-hue.mod"); err != nil || got != "v0.2.0" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestTail(t *testing.T) {
	var out []string
	for i := range 60 {
		out = append(out, strings.Repeat("x", i))
	}
	lines := strings.Split(tail([]byte(strings.Join(out, "\n")+"\n"), errors.New("exit status 1")), "\n")
	if len(lines) != tailLines+1 || lines[0] != strings.Repeat("x", 10) || lines[tailLines] != "exit status 1" {
		t.Errorf("tail: %q", lines)
	}
}

// Check turns what Build left into the index: a module whose executable
// lists its types is compatible, one listing others is rejected, one whose
// executable fails is not compatible, and one not built is kept as is.
func TestCheck(t *testing.T) {
	dir := t.TempDir()
	module := func(mod, repo string, types ...string) Module {
		m := Module{Module: mod, Repo: repoPrefix + repo, Types: map[string]Type{}, Versions: []Version{{Version: "v1.0.0"}}, Latest: "v1.0.0", Oiko: "v0.2.0"}
		for _, t := range types {
			m.Types[t] = Type{"A type", json.RawMessage(`{}`)}
		}
		return m
	}
	reused := module("example.com/reused", "someone/reused", "reused")
	reused.Compatible = true
	w := work{
		Index: Index{
			Modules: []Module{
				module("github.com/someone/oiko-hue", "someone/oiko-hue", "hue", "hue-sensor"),
				module("example.com/oiko-sonos", "someone/oiko-sonos", "sonos", "sonos-sub"),
				module("example.com/broken", "someone/broken", "broken"),
				reused,
			},
			Rejected: []Rejected{{repoPrefix + "someone/oiko-zwave", "no manifest"}},
		},
		Built: map[string]string{
			"github.com/someone/oiko-hue": script(t, dir, "hue", "printf '%s' '"+string(readFile(t, "testdata/version.txt"))+"'"),
			"example.com/oiko-sonos":      script(t, dir, "sonos", "printf '%s' '"+string(readFile(t, "testdata/version.txt"))+"'"),
			"example.com/broken":          script(t, dir, "broken", "echo 'flag provided but not defined: -version'; exit 2"),
		},
	}
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "work.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ix, err := Check(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseIndex(ix.Encode()); err != nil {
		t.Fatal(err)
	}
	got := map[string]Module{}
	for _, m := range ix.Modules {
		got[m.Module] = m
	}
	if m := got["github.com/someone/oiko-hue"]; !m.Compatible {
		t.Errorf("hue: %+v", m)
	}
	if m := got["example.com/broken"]; m.Compatible || !strings.Contains(m.Error, "flag provided but not defined") {
		t.Errorf("broken: %+v", m)
	}
	if m := got["example.com/reused"]; !m.Compatible {
		t.Errorf("reused: %+v", m)
	}
	if _, ok := got["example.com/oiko-sonos"]; ok || len(ix.Modules) != 3 {
		t.Errorf("modules: %+v", ix.Modules)
	}
	want := []Rejected{
		{repoPrefix + "someone/oiko-zwave", "no manifest"},
		{repoPrefix + "someone/oiko-sonos", "types mismatch: the manifest names sonos, sonos-sub; v1.0.0 registers sonos"},
	}
	if !slices.Equal(ix.Rejected, want) {
		t.Errorf("rejected: %+v", ix.Rejected)
	}
}

// script writes a shell script running body, standing for an Oiko.
func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
