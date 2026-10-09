// The catalogue site: the types of Bridge of index.json, and for those picked,
// how to build and run an Oiko with them (a binary, Docker, NixOS) and the
// bridges section of its configuration.
//
// The exported functions are pure, tested by site_test.js.

const builder = "github.com/llehouerou/oiko/cmd/oiko-build";

// types lists every indexed type, by name, then module path.
export function types(index) {
  return index.modules
    .flatMap((m) =>
      Object.entries(m.types).map(([name, t]) => ({
        name,
        description: t.description,
        config: t.config,
        module: m.module,
        repo: m.repo,
        license: m.license,
        latest: m.latest,
        oiko: m.oiko,
        compatible: m.compatible,
        error: m.error,
      })),
    )
    .sort((a, b) => compare(a.name, b.name) || compare(a.module, b.module));
}

// key identifies a type: Bridges are keyed by name, but two modules may
// provide a type of the same name.
export function key(type) {
  return `${type.module} ${type.name}`;
}

// blocked tells why type cannot be picked alongside picked, or "" if it can.
export function blocked(type, picked) {
  if (!type.compatible) {
    return `does not build with Oiko ${type.oiko}`;
  }
  const other = picked.find((p) => p.name === type.name && p.module !== type.module);
  return other ? `${other.name} from ${other.module} is picked` : "";
}

// command is the oiko-build command building an Oiko with the picked types,
// each module at its latest version, and the Oiko they were built against, or
// oiko when given.
export function command(picked, oiko) {
  return [
    `go run ${builder}@${oikoOf(picked, oiko)} \\`,
    ...modules(picked).map(([m, v]) => `  -with ${m}@${v} \\`),
    "  -o oiko",
  ].join("\n");
}

// oikoOf is the Oiko the picked types were built against, and oiko when given.
function oikoOf(picked, oiko) {
  const oikos = new Set(picked.map((t) => t.oiko));
  if (oiko) {
    oikos.add(oiko);
  }
  if (oikos.size !== 1) {
    throw new Error(`want one Oiko, got ${[...oikos].join(", ") || "none"}`);
  }
  return [...oikos][0];
}

// modules is the picked types' modules with their latest version, by path.
function modules(picked) {
  return [...new Map(picked.map((t) => [t.module, t.latest]))].sort(([a], [b]) => compare(a, b));
}

// dockerfile is a Dockerfile building an Oiko with the picked types. Its RUN
// holds command as is: Oiko's update banner names the line to replace.
export function dockerfile(picked, oiko) {
  return `FROM golang:1 AS build
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
WORKDIR /src
RUN ${command(picked, oiko)}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /src/oiko /oiko
ENV OIKO_INSTALL=docker
VOLUME /data
ENTRYPOINT ["/oiko", "-data", "/data"]
`;
}

// compose is the compose.yaml running the Dockerfile's image.
export function compose() {
  return `services:
  oiko:
    build: .
    network_mode: host
    user: "1000:1000"  # your id -u:id -g
    volumes:
      - ./data:/data
      - /etc/localtime:/etc/localtime:ro
    restart: unless-stopped
`;
}

// config is a data/config.json starting from the picked types' examples.
export function config(picked) {
  return JSON.stringify({ bridges: bridges(picked) }, null, 2);
}

// bridges is the bridges section of the configuration: the picked types'
// examples, by type name.
function bridges(picked) {
  const bridges = {};
  for (const t of [...picked].sort((a, b) => compare(a.name, b.name))) {
    if (t.name in bridges) {
      throw new Error(`${t.name} is picked twice`);
    }
    bridges[t.name] = t.config;
  }
  return bridges;
}

// flake is what a NixOS configuration's flake.nix needs: the oiko input,
// pinned to the release the picked types build against, and the modules.
export function flake(picked, oiko) {
  return `inputs.oiko.url = "github:llehouerou/oiko/${oikoOf(picked, oiko)}";

outputs = { nixpkgs, oiko, ... }: {
  nixosConfigurations.home = nixpkgs.lib.nixosSystem { # your host
    specialArgs = { inherit oiko; };
    modules = [ oiko.nixosModules.default ./oiko.nix ];
  };
};
`;
}

// oikoNix is the oiko.nix flake imports: the service, its package built with
// the picked types (the override lines are those of Oiko's update banner),
// and their examples as the bridges of its configuration.
export function oikoNix(picked) {
  return `{ oiko, pkgs, lib, ... }:
{
  services.oiko.enable = true;
  services.oiko.mqtt = "mqtt://localhost:1883"; # your zigbee2mqtt broker
  services.oiko.package = oiko.packages.\${pkgs.stdenv.hostPlatform.system}.default.override {
${modules(picked).map(([m, v]) => `    bridges.${nix(m)} = ${nix(v)};\n`).join("")}    vendorHash = lib.fakeHash; # the first build prints the hash to set
  };
  services.oiko.settings.bridges = ${nix(bridges(picked), "  ")};
}
`;
}

// nix is value, a JSON value, as a Nix expression, lines indented by indent.
export function nix(value, indent = "") {
  const inner = indent + "  ";
  if (value === null || typeof value === "boolean") {
    return String(value);
  }
  if (typeof value === "number") {
    let n = JSON.stringify(value);
    if (n.includes("e") && !n.includes(".")) {
      n = n.replace("e", ".0e"); // a Nix float has a dot
    }
    return value < 0 ? `(${n})` : n;
  }
  if (typeof value === "string") {
    const escapes = { "\\": "\\\\", '"': '\\"', "${": "\\${", "\n": "\\n", "\r": "\\r", "\t": "\\t" };
    return `"${value.replace(/\\|"|\$\{|\n|\r|\t/g, (c) => escapes[c])}"`;
  }
  if (Array.isArray(value)) {
    return value.length === 0 ? "[ ]" : `[\n${value.map((v) => `${inner}${nix(v, inner)}\n`).join("")}${indent}]`;
  }
  const entries = Object.entries(value);
  return entries.length === 0
    ? "{ }"
    : `{\n${entries.map(([k, v]) => `${inner}${attr(k)} = ${nix(v, inner)};\n`).join("")}${indent}}`;
}

const keywords = new Set(["assert", "else", "if", "in", "inherit", "let", "or", "rec", "then", "with"]);

function attr(name) {
  return /^[A-Za-z_][A-Za-z0-9_'-]*$/.test(name) && !keywords.has(name) ? name : nix(name);
}

function compare(a, b) {
  return a < b ? -1 : a > b ? 1 : 0;
}

// The page.

if (typeof document !== "undefined") {
  const response = await fetch("index.json");
  if (!response.ok) {
    throw new Error(`index.json: ${response.status}`);
  }
  const all = types(await response.json());
  const picked = new Map(); // by key
  const list = document.getElementById("types");
  const rows = all.map((type) => row(type, () => {
    if (picked.has(key(type))) {
      picked.delete(key(type));
    } else {
      picked.set(key(type), type);
    }
    render();
  }));
  list.replaceChildren(...rows.map((r) => r.element));
  for (const r of rows) {
    // shortcut: a name two modules provide anchors the first, by module path, and a name the
    // page's own ids take (docker, config…) none; rename the page's ids once such a type is listed.
    if (!document.getElementById(r.type.name)) {
      r.element.id = r.type.name;
    }
  }
  if (all.length === 0) {
    list.textContent = "No type of Bridge is indexed yet.";
  }
  if (location.hash) {
    location.replace(location.hash); // the rows came after the page's own scroll to its #<type>
  }
  for (const button of document.querySelectorAll("button[data-copy]")) {
    button.addEventListener("click", async () => {
      await navigator.clipboard.writeText(document.getElementById(button.dataset.copy).textContent);
      button.textContent = "Copied";
      setTimeout(() => (button.textContent = "Copy"), 1500);
    });
  }
  const tabs = [...document.querySelectorAll("button[role=tab]")];
  for (const tab of tabs) {
    tab.addEventListener("click", () => {
      for (const t of tabs) {
        t.setAttribute("aria-selected", t === tab);
        document.getElementById(t.getAttribute("aria-controls")).hidden = t !== tab;
      }
      document.getElementById("shared").hidden = tab.hasAttribute("data-own-config");
    });
  }
  document.getElementById("compose").textContent = compose();

  function render() {
    const chosen = [...picked.values()];
    for (const r of rows) {
      const why = blocked(r.type, chosen);
      r.input.checked = picked.has(key(r.type));
      r.input.disabled = why !== "";
      r.element.classList.toggle("blocked", why !== "");
      r.element.title = why;
    }
    document.getElementById("output").hidden = chosen.length === 0;
    document.getElementById("placeholder").hidden = chosen.length > 0;
    if (chosen.length > 0) {
      document.getElementById("command").textContent = command(chosen);
      document.getElementById("dockerfile").textContent = dockerfile(chosen);
      document.getElementById("config").textContent = config(chosen);
      document.getElementById("flake").textContent = flake(chosen);
      document.getElementById("oiko-nix").textContent = oikoNix(chosen);
    }
  }
  render();
}

function row(type, toggle) {
  const element = document.createElement("li");
  const label = document.createElement("label");
  const input = document.createElement("input");
  input.type = "checkbox";
  input.addEventListener("change", toggle);
  const name = document.createElement("strong");
  name.textContent = type.name;
  const description = document.createElement("span");
  description.textContent = type.description;
  label.append(input, " ", name, " ", description);

  const module = document.createElement("div");
  module.className = "module";
  const link = document.createElement("a");
  link.href = type.repo;
  link.textContent = `${type.module}@${type.latest}`;
  module.append(link, `, ${type.license}`, type.compatible ? `, builds with Oiko ${type.oiko}` : "");
  element.append(label, module);

  if (!type.compatible) {
    const details = document.createElement("details");
    const summary = document.createElement("summary");
    summary.textContent = `Does not build with Oiko ${type.oiko}`;
    const error = document.createElement("pre");
    error.textContent = type.error;
    details.append(summary, error);
    element.append(details);
  }
  return { type, element, input };
}
