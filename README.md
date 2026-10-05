# Oiko catalogue

The catalogue of types of Bridge for [Oiko](https://github.com/llehouerou/oiko): Go modules
others write, compiled into Oiko with `oiko-build`. [`index.json`](index.json) lists them,
rebuilt every day by [a workflow](.github/workflows/index.yml), and
[the site](https://llehouerou.github.io/oiko-catalogue/) shows them.

## Listing a type of Bridge

1. Give the repository the GitHub topic `oiko-bridge`.
2. Put the module at the repository's root, its root package registering every type it
   provides (`bridge.Register` in `init`): `oiko-build` imports that package.
3. Add `oiko-bridge.json`, the manifest below, at the module's root.
4. Tag a release (`v1.2.0`; pre-releases are left out). The catalogue reads the manifest of
   the latest release.

### `oiko-bridge.json`

```json
{
  "types": {
    "hue": {
      "description": "Philips Hue lights through a Hue Bridge",
      "config": {"host": "192.168.1.10"}
    }
  }
}
```

- `types`: each type the root package registers, by the name it registers it under: lowercase
  letters, digits and dashes. At least one; exactly those it registers.
- `description`: what the type connects Oiko to, in a sentence. Required.
- `config`: an example of the type's section of `bridges` in Oiko's configuration, its body: a
  JSON object, `{}` when the type needs nothing. The catalogue shows it as the section named
  after the type, `"bridges": {"hue": {"host": "192.168.1.10"}}`. A `type` key in it, if any,
  must be the type's name.

Any other field is refused. There is no minimum Oiko version: it is the version of
`github.com/llehouerou/oiko` the module's `go.mod` requires.

## The index

Every day, and on demand, the workflow:

1. searches GitHub for the repositories with the topic, and reads the module path of each from
   the `go.mod` at its root;
2. lists the module's versions from the Go module proxy, pre-releases left out, and the Oiko
   each one's `go.mod` requires;
3. reads the manifest of the latest release;
4. builds that release against the latest Oiko release,
   `go run github.com/llehouerou/oiko/cmd/oiko-build@<oiko> -with <module>@<latest>`, unless
   the previous index holds that pair already (or the run is forced), and runs the result's
   `-version`: the types it lists as added from the module must be the manifest's.

```json
{
  "modules": [
    {
      "module": "github.com/someone/oiko-hue",
      "repo": "https://github.com/someone/oiko-hue",
      "types": {
        "hue": {
          "description": "Philips Hue lights through a Hue Bridge",
          "config": {"host": "192.168.1.10"}
        }
      },
      "versions": [
        {"version": "v1.1.0", "minOiko": "v0.1.0"},
        {"version": "v1.2.0", "minOiko": "v0.2.0"}
      ],
      "latest": "v1.2.0",
      "oiko": "v0.2.0",
      "compatible": true
    }
  ],
  "rejected": [
    {"repo": "https://github.com/someone/oiko-zwave", "reason": "no manifest"}
  ]
}
```

- `modules`: the modules indexed, by module path. `types` are the latest release's, `oiko` the
  Oiko release it was built against, `compatible` whether it builds and runs, and `error`, when
  not, the end of the build's output. `minOiko` is absent when a version's `go.mod` requires no
  Oiko.
- `rejected`: the repositories with the topic not indexed, and why: no release, no or an
  invalid manifest, a module not at the root, or types that differ from those registered.

Building a type runs code nobody reviewed: the job that builds and runs it may only read, and
runs it once Oiko's credentials are gone; another job validates the index and commits it.

## Site

[`site/`](site) is the site, published on GitHub Pages by [a workflow](.github/workflows/pages.yml)
whenever it or `index.json` changes on `main`. It lists the indexed types; the visitor picks
some and gets the `oiko-build` command building an Oiko with them, each module at its latest
version, and a `data/config.json` with their sections of `bridges`, from their manifests'
`config`. A static page and one ES module, no build step: `site/index.json` is a link to the
index.

The Docker tab gives instead a `Dockerfile` and a `compose.yaml`. The Dockerfile builds in
`golang:1`, its `RUN` holding the `oiko-build` command as is (Oiko's update banner names the
one to put in its place), and runs Oiko in `gcr.io/distroless/static-debian12:nonroot` with
`OIKO_INSTALL=docker` and its data in the volume `/data`. compose.yaml runs it on the host's
network (HomeKit discovers accessories over mDNS), as the occupant's user, with `./data` and the
host's timezone mounted. [Another workflow](.github/workflows/docker.yml) builds the pair for no
type against the latest Oiko and checks its dashboard answers, on every change to the site.

The NixOS tab gives what a flake-based NixOS configuration adds: in `flake.nix`, the `oiko`
input pinned to the Oiko release the types build against and its module; and an `oiko.nix`
enabling the service with the package overridden with each module at its latest version (the
lines Oiko's update banner shows; `vendorHash` is set from the first build's failure), and the
examples as `services.oiko.settings.bridges`, written as Nix. The shared `data/config.json` is
hidden there. [A third workflow](.github/workflows/nixos.yml) evaluates the pair for fixture
types against the latest Oiko: the package's derivation and the bridges of `config.json`.

## Development

```sh
nix develop          # or Go, the version go.mod names, and Node
go test ./...
node --test          # the site's outputs, against testdata/index.json
python3 -m http.server -d site   # the site, on http://localhost:8000
go run . build -work /tmp/work [-force]   # GITHUB_TOKEN for the search
go run . check -work /tmp/work -index index.json
go run . validate -index index.json
```
