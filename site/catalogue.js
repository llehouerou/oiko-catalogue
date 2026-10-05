// The catalogue site: the types of Bridge of index.json, and for those picked,
// the oiko-build command and the bridges section of data/config.json.
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
  const oikos = new Set(picked.map((t) => t.oiko));
  if (oiko) {
    oikos.add(oiko);
  }
  if (oikos.size !== 1) {
    throw new Error(`want one Oiko, got ${[...oikos].join(", ") || "none"}`);
  }
  const modules = new Map(picked.map((t) => [t.module, t.latest]));
  return [
    `go run ${builder}@${[...oikos][0]} \\`,
    ...[...modules.keys()].sort(compare).map((m) => `  -with ${m}@${modules.get(m)} \\`),
    "  -o oiko",
  ].join("\n");
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
  const bridges = {};
  for (const t of [...picked].sort((a, b) => compare(a.name, b.name))) {
    if (t.name in bridges) {
      throw new Error(`${t.name} is picked twice`);
    }
    bridges[t.name] = t.config;
  }
  return JSON.stringify({ bridges }, null, 2);
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
  if (all.length === 0) {
    list.textContent = "No type of Bridge is indexed yet.";
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
  module.append(link, type.compatible ? ` builds with Oiko ${type.oiko}` : "");
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
