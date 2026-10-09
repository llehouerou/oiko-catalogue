# Contributing

This repository is the catalogue: the indexer, `index.json` and the site. A type of Bridge is
listed from its own repository, with no issue or pull request here: see
[Listing a type of Bridge](README.md#listing-a-type-of-bridge). The Manifest's spec,
[`oiko-bridge.json`](README.md#oiko-bridgejson), lives in the README, and changes there.

## Build and test

```sh
nix develop          # or Go, the version go.mod names, and Node 24
go test ./...        # the indexer, the index and the manifest
node --test          # the site's outputs, against testdata/index.json
python3 -m http.server -d site   # the site, on http://localhost:8000
```

CI runs `node --test` before publishing the site, and checks its Docker and NixOS outputs
against the latest Oiko.

## Everything else

[Oiko's CONTRIBUTING](https://github.com/llehouerou/oiko/blob/main/CONTRIBUTING.md) applies here:
where questions and ideas go, when an issue comes first, the pull request terms, AI-assisted
contributions and response times. Everyone taking part follows
[Oiko's code of conduct](https://github.com/llehouerou/oiko/blob/main/CODE_OF_CONDUCT.md).

The catalogue is under Apache-2.0 ([LICENSE](LICENSE), [NOTICE](NOTICE)), and a contribution comes
in under the same license (ADR 0047 of Oiko).
