import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { blocked, command, compose, config, dockerfile, flake, key, nix, oikoNix, types } from "./site/catalogue.js";

const all = types(JSON.parse(readFileSync(new URL("testdata/index.json", import.meta.url))));
const pick = (...keys) => keys.map((k) => all.find((t) => key(t) === k));

const hue = "github.com/someone/oiko-hue hue";
const hueSensor = "github.com/someone/oiko-hue hue-sensor";
const lightsHue = "github.com/someone/oiko-lights hue";
const nanoleaf = "github.com/someone/oiko-lights nanoleaf";
const sonos = "example.com/oiko-sonos sonos";

test("types lists every indexed type, by name then module", () => {
  assert.deepEqual(all.map(key), [hue, lightsHue, hueSensor, nanoleaf, sonos]);
  assert.deepEqual(all[0], {
    name: "hue",
    description: "Philips Hue lights through a Hue Bridge",
    config: { host: "192.168.1.10" },
    module: "github.com/someone/oiko-hue",
    repo: "https://github.com/someone/oiko-hue",
    latest: "v1.2.0",
    oiko: "v0.2.0",
    compatible: true,
    error: undefined,
  });
});

test("blocked: incompatible types, and same-name types of other modules once one is picked", () => {
  const [h, lh, , n, s] = pick(hue, lightsHue, hueSensor, nanoleaf, sonos);
  assert.equal(blocked(s, []), "does not build with Oiko v0.2.0");
  assert.equal(blocked(h, []), "");
  assert.equal(blocked(lh, [h]), "hue from github.com/someone/oiko-hue is picked");
  assert.equal(blocked(h, [h]), "");
  assert.equal(blocked(n, [h]), "");
});

test("command: one -with per module, sorted by module path", () => {
  assert.equal(
    command(pick(nanoleaf, hueSensor, hue)),
    [
      "go run github.com/llehouerou/oiko/cmd/oiko-build@v0.2.0 \\",
      "  -with github.com/someone/oiko-hue@v1.2.0 \\",
      "  -with github.com/someone/oiko-lights@v0.3.0 \\",
      "  -o oiko",
    ].join("\n"),
  );
});

test("command refuses types built against different Oikos, or none", () => {
  const [h, n] = pick(hue, nanoleaf);
  assert.throws(() => command([h, { ...n, oiko: "v0.3.0" }]), /want one Oiko, got v0.2.0, v0.3.0/);
  assert.throws(() => command([h], "v0.3.0"), /want one Oiko/);
  assert.throws(() => command([]), /want one Oiko, got none/);
});

test("command with an explicit Oiko", () => {
  assert.equal(command([], "v0.2.0"), "go run github.com/llehouerou/oiko/cmd/oiko-build@v0.2.0 \\\n  -o oiko");
  assert.equal(command(pick(hue), "v0.2.0"), command(pick(hue)));
});

test("dockerfile: its RUN holds the command as is", () => {
  assert.equal(
    dockerfile(pick(nanoleaf, hue)),
    `FROM golang:1 AS build
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
WORKDIR /src
RUN go run github.com/llehouerou/oiko/cmd/oiko-build@v0.2.0 \\
  -with github.com/someone/oiko-hue@v1.2.0 \\
  -with github.com/someone/oiko-lights@v0.3.0 \\
  -o oiko

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /src/oiko /oiko
ENV OIKO_INSTALL=docker
VOLUME /data
ENTRYPOINT ["/oiko", "-data", "/data"]
`,
  );
  assert.ok(dockerfile([], "v0.2.0").includes(`RUN ${command([], "v0.2.0")}\n`));
});

test("compose", () => {
  assert.equal(
    compose(),
    `services:
  oiko:
    build: .
    network_mode: host
    user: "1000:1000"  # your id -u:id -g
    volumes:
      - ./data:/data
      - /etc/localtime:/etc/localtime:ro
    restart: unless-stopped
`,
  );
});

test("config: the examples, by type name", () => {
  assert.equal(
    config(pick(hueSensor, hue)),
    `{
  "bridges": {
    "hue": {
      "host": "192.168.1.10"
    },
    "hue-sensor": {
      "type": "hue-sensor",
      "host": "192.168.1.10"
    }
  }
}`,
  );
  assert.throws(() => config(pick(hue, lightsHue)), /hue is picked twice/);
});

test("flake: the oiko input at the picked types' Oiko, and the modules", () => {
  assert.equal(
    flake(pick(nanoleaf, hue)),
    `inputs.oiko.url = "github:llehouerou/oiko/v0.2.0";

outputs = { nixpkgs, oiko, ... }: {
  nixosConfigurations.home = nixpkgs.lib.nixosSystem { # your host
    specialArgs = { inherit oiko; };
    modules = [ oiko.nixosModules.default ./oiko.nix ];
  };
};
`,
  );
  assert.ok(flake([], "v0.3.0").startsWith('inputs.oiko.url = "github:llehouerou/oiko/v0.3.0";\n'));
  const [h, n] = pick(hue, nanoleaf);
  assert.throws(() => flake([h, { ...n, oiko: "v0.3.0" }]), /want one Oiko/);
});

test("oikoNix: the package with each module, and the examples as Nix", () => {
  assert.equal(
    oikoNix(pick(nanoleaf, hueSensor, hue)),
    String.raw`{ oiko, pkgs, lib, ... }:
{
  services.oiko.enable = true;
  services.oiko.mqtt = "mqtt://localhost:1883"; # your zigbee2mqtt broker
  services.oiko.package = oiko.packages.${"$"}{pkgs.stdenv.hostPlatform.system}.default.override {
    bridges."github.com/someone/oiko-hue" = "v1.2.0";
    bridges."github.com/someone/oiko-lights" = "v0.3.0";
    vendorHash = lib.fakeHash; # the first build prints the hash to set
  };
  services.oiko.settings.bridges = {
    hue = {
      host = "192.168.1.10";
    };
    hue-sensor = {
      type = "hue-sensor";
      host = "192.168.1.10";
    };
    nanoleaf = {
      host = "192.168.1.30";
      token = "a\"b\\c\${d}\n";
      panels = {
        "1st" = 1;
        "living room" = [
          true
          null
          (-2.5)
        ];
        "or" = { };
      };
    };
  };
}
`,
  );
});

test("nix: scalars, escapes and keys", () => {
  assert.equal(nix("plain"), '"plain"');
  assert.equal(nix('\\ " ${x} $y \n\r\t'), String.raw`"\\ \" \${x} $y \n\r\t"`);
  assert.equal(nix(0), "0");
  assert.equal(nix(-3), "(-3)");
  assert.equal(nix(1e21), "1.0e+21");
  assert.equal(nix(1.5e-7), "1.5e-7");
  assert.equal(nix(false), "false");
  assert.equal(nix([]), "[ ]");
  assert.equal(nix({}), "{ }");
  assert.equal(
    nix({ a_b: 1, "c'-d": 2, "-e": 3, "f.g": 4, "": 5, if: 6, 'h"': 7 }),
    String.raw`{
  a_b = 1;
  c'-d = 2;
  "-e" = 3;
  "f.g" = 4;
  "" = 5;
  "if" = 6;
  "h\"" = 7;
}`,
  );
});
