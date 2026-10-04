package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const (
	oikoModule = "github.com/llehouerou/oiko"
	oikoBuild  = oikoModule + "/cmd/oiko-build"
	topic      = "oiko-bridge"
)

// work is what Build hands Check, in <work>/work.json: the index built, and
// the executables of the modules whose types are still to be checked.
type work struct {
	Index Index             `json:"index"`
	Built map[string]string `json:"built"` // module → executable
}

// Build indexes the repositories with the topic, building the latest version
// of each module against the latest Oiko unless prev holds that pair already
// (or force), and writes what Check needs to dir. It runs nothing of the
// types: Check does, once credentials are gone.
func Build(ctx context.Context, prev Index, dir string, force bool) error {
	oiko, err := latestOiko(ctx, dir)
	if err != nil {
		return err
	}
	repos, err := search(ctx)
	if err != nil {
		return err
	}
	previous := map[string]Module{}
	known := map[string]string{} // module@version → minimum Oiko
	for _, m := range prev.Modules {
		previous[m.Module] = m
		for _, v := range m.Versions {
			known[m.Module+"@"+v.Version] = v.MinOiko
		}
	}
	w := work{Built: map[string]string{}}
	indexedFrom := map[string]string{} // module → repo
	for i, r := range repos {
		repo := repoPrefix + r.FullName
		m, reason, err := resolve(ctx, dir, r, known)
		if err != nil {
			return fmt.Errorf("%s: %w", repo, err)
		}
		if other, ok := indexedFrom[m.Module]; ok && reason == "" {
			reason = fmt.Sprintf("module not at root: %s is indexed from %s", m.Module, other)
		}
		if reason != "" {
			w.Index.Rejected = append(w.Index.Rejected, Rejected{repo, reason})
			continue
		}
		indexedFrom[m.Module] = repo
		m.Repo, m.Oiko = repo, oiko
		if p, ok := previous[m.Module]; ok && !force && p.Latest == m.Latest && p.Oiko == oiko {
			m.Compatible, m.Error = p.Compatible, p.Error
		} else {
			exe := filepath.Join(dir, strconv.Itoa(i), "oiko")
			out, err := goCmd(ctx, dir, "run", oikoBuild+"@"+oiko, "-with", m.Module+"@"+m.Latest, "-o", exe)
			if err != nil {
				m.Error = tail(out, err)
			} else {
				w.Built[m.Module] = exe
			}
		}
		w.Index.Modules = append(w.Index.Modules, m)
	}
	data, err := json.Marshal(w)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "work.json"), data, 0o644)
}

// Check runs the -version of each executable Build left in dir, and returns
// the index: a module is compatible when its executable runs and lists as
// added from it the types its manifest names; listing others rejects it.
func Check(ctx context.Context, dir string) (Index, error) {
	data, err := os.ReadFile(filepath.Join(dir, "work.json"))
	if err != nil {
		return Index{}, err
	}
	var w work
	if err := json.Unmarshal(data, &w); err != nil {
		return Index{}, err
	}
	ix := Index{Rejected: w.Index.Rejected}
	for _, m := range w.Index.Modules {
		if exe, ok := w.Built[m.Module]; ok {
			out, err := runVersion(ctx, exe)
			if err != nil {
				m.Error = tail(out, err)
			} else if got, want := addedTypes(out, m.Module), slices.Sorted(maps.Keys(m.Types)); !slices.Equal(got, want) {
				ix.Rejected = append(ix.Rejected, Rejected{m.Repo, fmt.Sprintf("types mismatch: the manifest names %s; %s registers %s",
					list(want), m.Latest, list(got))})
				continue
			} else {
				m.Compatible = true
			}
		}
		ix.Modules = append(ix.Modules, m)
	}
	return ix, ix.Check()
}

// resolve reads the module of repository r: its path from the go.mod at the
// root of its default branch, its releases with the Oiko each requires, and
// the types of the manifest of the latest. A reason is given instead when the
// repository is not indexed. known holds the Oiko required by releases
// already indexed, by module@version: a release does not change.
func resolve(ctx context.Context, dir string, r repository, known map[string]string) (m Module, reason string, err error) {
	gomod, err := fetch(ctx, "https://raw.githubusercontent.com/"+r.FullName+"/"+r.DefaultBranch+"/go.mod")
	if errors.Is(err, fs.ErrNotExist) {
		return m, "module not at root: no go.mod", nil
	}
	if err != nil {
		return m, "", err
	}
	m.Module = modfile.ModulePath(gomod)
	if module.CheckPath(m.Module) != nil {
		return m, fmt.Sprintf("module not at root: go.mod declares %q", m.Module), nil
	}
	if prefix, _, _ := module.SplitPathVersion(m.Module); strings.HasPrefix(m.Module, "github.com/") && !strings.EqualFold(prefix, "github.com/"+r.FullName) {
		return m, "module not at root: go.mod declares " + m.Module, nil
	}

	var listed struct{ Versions []string }
	if out, err := goCmd(ctx, dir, "list", "-m", "-versions", "-json", m.Module); err != nil {
		return m, "no release: " + lastLine(out, err), nil
	} else if err := json.Unmarshal(out, &listed); err != nil {
		return m, "", err
	}
	var queries []string
	for _, v := range listed.Versions {
		if min, ok := known[m.Module+"@"+v]; ok {
			m.Versions = append(m.Versions, Version{v, min})
		} else if release(v) {
			queries = append(queries, m.Module+"@"+v)
		}
	}
	if len(m.Versions)+len(queries) == 0 {
		return m, "no release", nil
	}
	if len(queries) > 0 {
		out, err := goCmd(ctx, dir, append([]string{"list", "-m", "-json"}, queries...)...)
		if err != nil {
			return m, "invalid release: " + lastLine(out, err), nil
		}
		for d := json.NewDecoder(bytes.NewReader(out)); d.More(); {
			var v struct{ Version, GoMod string }
			if err := d.Decode(&v); err != nil {
				return m, "", err
			}
			min, err := minOiko(v.GoMod)
			if err != nil {
				return m, "invalid release: " + err.Error(), nil
			}
			m.Versions = append(m.Versions, Version{v.Version, min})
		}
	}
	slices.SortFunc(m.Versions, func(a, b Version) int { return semver.Compare(a.Version, b.Version) })
	m.Latest = m.Versions[len(m.Versions)-1].Version

	var downloaded struct{ Dir string }
	if out, err := goCmd(ctx, dir, "mod", "download", "-json", m.Module+"@"+m.Latest); err != nil {
		return m, "invalid release: " + lastLine(out, err), nil
	} else if err := json.Unmarshal(out, &downloaded); err != nil {
		return m, "", err
	}
	manifest, err := os.ReadFile(filepath.Join(downloaded.Dir, ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return m, "no manifest", nil
	}
	if err != nil {
		return m, "", err
	}
	if m.Types, err = ParseManifest(manifest); err != nil {
		return m, "invalid manifest: " + err.Error(), nil
	}
	return m, "", nil
}

// minOiko is the version of Oiko the go.mod at path requires.
func minOiko(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	f, err := modfile.ParseLax(path, data, nil)
	if err != nil {
		return "", err
	}
	for _, r := range f.Require {
		if r.Mod.Path == oikoModule {
			return r.Mod.Version, nil
		}
	}
	return "", nil
}

func latestOiko(ctx context.Context, dir string) (string, error) {
	var latest struct{ Version string }
	out, err := goCmd(ctx, dir, "list", "-m", "-json", oikoModule+"@latest")
	if err != nil {
		return "", fmt.Errorf("latest Oiko: %s", lastLine(out, err))
	}
	if err := json.Unmarshal(out, &latest); err != nil {
		return "", err
	}
	if !release(latest.Version) {
		return "", fmt.Errorf("latest Oiko: %q is not a release", latest.Version)
	}
	return latest.Version, nil
}

// goCmd runs go in dir, returning its output, standard error included when
// it fails.
func goCmd(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return append(stdout.Bytes(), stderr.Bytes()...), fmt.Errorf("go %s: %w", args[0], err)
	}
	return stdout.Bytes(), nil
}

// runVersion runs exe -version, with no environment.
func runVersion(ctx context.Context, exe string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-version")
	cmd.Dir, cmd.Env, cmd.WaitDelay = filepath.Dir(exe), []string{}, 5*time.Second
	return cmd.CombinedOutput()
}

// addedTypes are the types an Oiko's -version lists as added from module
// mod, sorted. Its lines after the first are: type, origin, package, module,
// version.
func addedTypes(version []byte, mod string) []string {
	types := []string{}
	lines := strings.Split(strings.TrimSpace(string(version)), "\n")
	for _, l := range lines[1:] {
		if f := strings.Fields(l); len(f) == 5 && f[1] == "added" && f[3] == mod {
			types = append(types, f[0])
		}
	}
	slices.Sort(types)
	return types
}

// tailLines is how much of a failed build's output the index keeps.
const tailLines = 50

// tail is the end of the output of a command that failed with err.
func tail(out []byte, err error) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	lines = append(lines[max(0, len(lines)-tailLines):], err.Error())
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func lastLine(out []byte, err error) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return cmp.Or(lines[len(lines)-1], err.Error())
}

func list(types []string) string {
	if len(types) == 0 {
		return "none"
	}
	return strings.Join(types, ", ")
}

type repository struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
}

// search lists the repositories with the topic, sorted by name.
func search(ctx context.Context) ([]repository, error) {
	var repos []repository
	for page := 1; ; page++ {
		data, err := fetch(ctx, fmt.Sprintf("https://api.github.com/search/repositories?q=topic:%s&per_page=100&page=%d", topic, page))
		if err != nil {
			return nil, err
		}
		var res struct {
			Incomplete bool         `json:"incomplete_results"`
			Items      []repository `json:"items"`
		}
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, err
		}
		if res.Incomplete {
			return nil, errors.New("GitHub search: incomplete results")
		}
		repos = append(repos, res.Items...)
		if len(res.Items) < 100 {
			break
		}
	}
	slices.SortFunc(repos, func(a, b repository) int { return strings.Compare(a.FullName, b.FullName) })
	return slices.CompactFunc(repos, func(a, b repository) bool { return a.FullName == b.FullName }), nil
}

var client = &http.Client{Timeout: time.Minute}

// fetch gets url, with $GITHUB_TOKEN if set, which only the GitHub API
// needs; fs.ErrNotExist on 404.
func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" && strings.HasPrefix(url, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s: %w", url, fs.ErrNotExist)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return body.Bytes(), nil
}
