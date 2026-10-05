import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { blocked, command, config, key, types } from "./site/catalogue.js";

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

test("command refuses types built against different Oikos", () => {
  const [h, n] = pick(hue, nanoleaf);
  assert.throws(() => command([h, { ...n, oiko: "v0.3.0" }]), /different Oikos/);
});

test("config: the examples, by type name", () => {
  assert.equal(
    config(pick(nanoleaf, hueSensor, hue)),
    `{
  "bridges": {
    "hue": {
      "host": "192.168.1.10"
    },
    "hue-sensor": {
      "type": "hue-sensor",
      "host": "192.168.1.10"
    },
    "nanoleaf": {
      "host": "192.168.1.30",
      "token": ""
    }
  }
}`,
  );
  assert.throws(() => config(pick(hue, lightsHue)), /hue is picked twice/);
});
