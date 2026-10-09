package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// Index is the catalogue, index.json: the types of Bridge indexed, and the
// repositories with the topic that are not, with why.
type Index struct {
	Modules  []Module   `json:"modules"`
	Rejected []Rejected `json:"rejected"`
}

// Module is an indexed module of types of Bridge.
type Module struct {
	Module   string          `json:"module"`
	Repo     string          `json:"repo"`
	License  string          `json:"license"` // the SPDX id GitHub detects on Repo
	Types    map[string]Type `json:"types"`   // from the latest version's manifest
	Versions []Version       `json:"versions"`
	Latest   string          `json:"latest"`
	// Oiko is the release of Oiko Latest was built against.
	Oiko       string `json:"oiko"`
	Compatible bool   `json:"compatible"`
	Error      string `json:"error,omitempty"` // the tail of the build's output when not Compatible
}

// Version is a release of a module, with the version of Oiko its go.mod
// requires.
type Version struct {
	Version string `json:"version"`
	MinOiko string `json:"minOiko,omitempty"` // "" when its go.mod requires no Oiko
}

// Rejected is a repository with the topic that is not indexed.
type Rejected struct {
	Repo   string `json:"repo"`
	Reason string `json:"reason"`
}

const repoPrefix = "https://github.com/"

// Encode is ix as index.json holds it, modules and rejected sorted.
func (ix Index) Encode() []byte {
	ix.Modules = slices.Clone(ix.Modules)
	ix.Rejected = slices.Clone(ix.Rejected)
	slices.SortFunc(ix.Modules, func(a, b Module) int { return strings.Compare(a.Module, b.Module) })
	slices.SortFunc(ix.Rejected, func(a, b Rejected) int { return strings.Compare(a.Repo, b.Repo) })
	if ix.Modules == nil {
		ix.Modules = []Module{}
	}
	if ix.Rejected == nil {
		ix.Rejected = []Rejected{}
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	if err := e.Encode(ix); err != nil {
		panic(err) // the configs were checked to be JSON
	}
	return b.Bytes()
}

// ParseIndex reads index.json, refusing anything but an index Encode wrote
// and Check accepts: it is what the second job checks before committing, as
// the first ran code of the types.
func ParseIndex(data []byte) (Index, error) {
	var ix Index
	if err := decodeStrict(data, &ix); err != nil {
		return Index{}, err
	}
	if err := ix.Check(); err != nil {
		return Index{}, err
	}
	if !bytes.Equal(data, ix.Encode()) {
		return Index{}, errors.New("not in its canonical form")
	}
	return ix, nil
}

// Check checks the shape of ix: what it says may be turned into build
// commands and links.
func (ix Index) Check() error {
	seen := map[string]bool{}
	for _, m := range ix.Modules {
		if err := m.check(); err != nil {
			return fmt.Errorf("module %q: %w", m.Module, err)
		}
		if seen[m.Module] {
			return fmt.Errorf("module %q: listed twice", m.Module)
		}
		seen[m.Module] = true
	}
	for _, r := range ix.Rejected {
		if !validRepo(r.Repo) || r.Reason == "" {
			return fmt.Errorf("rejected %q: a repository and a reason are needed", r.Repo)
		}
	}
	return nil
}

func (m Module) check() error {
	if err := module.CheckPath(m.Module); err != nil {
		return err
	}
	if !validRepo(m.Repo) {
		return fmt.Errorf("repo %q is not a GitHub repository", m.Repo)
	}
	if !spdxID(m.License) {
		return fmt.Errorf("license %q is not an SPDX id", m.License)
	}
	if err := checkTypes(m.Types); err != nil {
		return err
	}
	if len(m.Versions) == 0 {
		return errors.New("no versions")
	}
	for i, v := range m.Versions {
		if !release(v.Version) || (v.MinOiko != "" && !canonical(v.MinOiko)) {
			return fmt.Errorf("version %q: want a release, and Oiko's version", v.Version)
		}
		if i > 0 && semver.Compare(m.Versions[i-1].Version, v.Version) >= 0 {
			return errors.New("versions not in ascending order")
		}
	}
	if m.Latest != m.Versions[len(m.Versions)-1].Version {
		return fmt.Errorf("latest %q is not the last version", m.Latest)
	}
	if !release(m.Oiko) {
		return fmt.Errorf("oiko %q is not a release", m.Oiko)
	}
	if m.Compatible != (m.Error == "") {
		return errors.New("an error is given when, and only when, it is not compatible")
	}
	return nil
}

// release reports whether v is a canonical semantic version and not a
// pre-release.
func release(v string) bool {
	return canonical(v) && semver.Prerelease(v) == ""
}

// canonical reports whether v is a version as Go writes it.
func canonical(v string) bool {
	return semver.IsValid(v) && semver.Canonical(v) == strings.TrimSuffix(v, "+incompatible")
}

func validRepo(repo string) bool {
	owner, name, ok := strings.Cut(strings.TrimPrefix(repo, repoPrefix), "/")
	return strings.HasPrefix(repo, repoPrefix) && ok && githubName(owner) && githubName(name)
}

func spdxID(s string) bool {
	return s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-.+") == ""
}

func githubName(s string) bool {
	return s != "" && s != "." && s != ".." && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.") == ""
}
