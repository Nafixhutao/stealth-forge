// Public status page behaviour: render the owner-published status page from the
// public endpoint. Everything rendered here comes from `GET /v1/status-page`;
// the page never authenticates and never reads an admin surface.

const ENDPOINT = "/v1/status-page";
const REFRESH_MS = 60_000;

const STATUS_LABELS = {
  operational: "Operational",
  degraded: "Degraded",
  partial_outage: "Partial outage",
  major_outage: "Major outage",
  maintenance: "Maintenance",
};

const HEADLINES = {
  operational: "All systems operational",
  degraded: "Degraded performance",
  partial_outage: "Partial outage",
  major_outage: "Major outage",
  maintenance: "Scheduled maintenance",
};

// Worst-first. Maintenance outranks a healthy component but never outranks a
// real fault, which is what a reader expects from a single summary line.
const SEVERITY_ORDER = [
  "operational",
  "maintenance",
  "degraded",
  "partial_outage",
  "major_outage",
];

const TIME_FORMAT = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

const el = (id) => document.getElementById(id);

const nodes = {
  summary: el("summary"),
  headline: el("summary-headline"),
  meta: el("summary-meta"),
  componentsPanel: el("components-panel"),
  components: el("components"),
  componentsHint: el("components-hint"),
  incidentsPanel: el("incidents-panel"),
  incidents: el("incidents"),
  incidentsHint: el("incidents-hint"),
  error: el("error"),
  footerNote: el("footer-note"),
  footerUpdated: el("footer-updated"),
  siteName: document.querySelector("[data-site-name]"),
  componentTemplate: el("component-template"),
  incidentTemplate: el("incident-template"),
};

function overallStatus(components) {
  let worst = "operational";
  for (const component of components) {
    const status = component.status;
    if (!SEVERITY_ORDER.includes(status)) continue;
    if (SEVERITY_ORDER.indexOf(status) > SEVERITY_ORDER.indexOf(worst)) {
      worst = status;
    }
  }
  return worst;
}

function activeIncidents(incidents) {
  return incidents.filter((incident) => incident.status !== "resolved");
}

function plural(count, noun) {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

function formatTime(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : TIME_FORMAT.format(date);
}

function renderComponents(components) {
  nodes.components.textContent = "";
  for (const component of components) {
    const row = nodes.componentTemplate.content.firstElementChild.cloneNode(true);
    const name = row.querySelector("[data-name]");
    const description = row.querySelector("[data-description]");
    const status = row.querySelector("[data-status]");
    const statusLabel = row.querySelector("[data-status-label]");

    name.textContent = component.name;
    if (typeof component.url === "string" && /^https?:\/\//.test(component.url)) {
      name.href = component.url;
      name.rel = "noreferrer";
    } else {
      name.replaceWith(document.createTextNode(component.name));
    }

    if (component.description) {
      description.textContent = component.description;
    } else {
      description.remove();
    }

    const key = STATUS_LABELS[component.status] ? component.status : "operational";
    status.dataset.status = key;
    statusLabel.textContent = STATUS_LABELS[key];
    nodes.components.append(row);
  }
  nodes.components.setAttribute("aria-busy", "false");
  nodes.componentsHint.textContent = plural(components.length, "component");
}

function renderIncidents(incidents) {
  nodes.incidents.textContent = "";
  if (incidents.length === 0) {
    nodes.incidentsPanel.hidden = true;
    return;
  }

  // Open incidents first, then the most recent resolved ones.
  const ordered = [
    ...activeIncidents(incidents),
    ...incidents.filter((incident) => incident.status === "resolved"),
  ];

  for (const incident of ordered) {
    const item = nodes.incidentTemplate.content.firstElementChild.cloneNode(true);
    const severity = item.querySelector("[data-severity]");
    const status = item.querySelector("[data-status]");
    const started = item.querySelector("[data-started]");
    const title = item.querySelector("[data-title]");
    const services = item.querySelector("[data-services]");
    const resolved = item.querySelector("[data-resolved]");

    severity.dataset.severity = incident.severity;
    severity.textContent = incident.severity;
    status.textContent = incident.status;
    started.textContent = formatTime(incident.started_at);
    title.textContent = incident.title;

    if (Array.isArray(incident.services) && incident.services.length > 0) {
      services.textContent = `Affected: ${incident.services.join(", ")}`;
    } else {
      services.remove();
    }

    if (incident.resolved_at) {
      resolved.textContent = `Resolved ${formatTime(incident.resolved_at)}`;
      resolved.hidden = false;
    }

    nodes.incidents.append(item);
  }

  const open = activeIncidents(incidents).length;
  nodes.incidentsHint.textContent =
    open === 0 ? "No open incidents" : `${plural(open, "open incident")}`;
  nodes.incidentsPanel.hidden = false;
}

function renderPublished(page) {
  const components = Array.isArray(page.components) ? page.components : [];
  const incidents = Array.isArray(page.incidents) ? page.incidents : [];
  const open = activeIncidents(incidents).length;
  const state = overallStatus(components);

  nodes.summary.dataset.state = state;
  nodes.headline.textContent = HEADLINES[state];
  nodes.meta.textContent = [
    `Updated ${formatTime(page.updated_at)}`,
    open === 0 ? "No open incidents" : plural(open, "open incident"),
  ].join(" · ");

  if (typeof page.name === "string" && page.name.trim() !== "") {
    nodes.siteName.textContent = page.name;
    document.title = `${page.name} · Status`;
  }

  const lead = el("page-description");
  if (typeof page.description === "string" && page.description.trim() !== "") {
    lead.textContent = page.description;
    lead.hidden = false;
  } else {
    lead.hidden = true;
  }

  renderComponents(components);
  renderIncidents(incidents);
  nodes.error.hidden = true;
}

function renderUnpublished() {
  nodes.summary.dataset.state = "none";
  nodes.headline.textContent = "Status page not published";
  nodes.meta.textContent = "This instance has not published a status page yet.";
  nodes.componentsPanel.hidden = true;
  nodes.incidentsPanel.hidden = true;
  nodes.error.hidden = true;
  nodes.footerUpdated.textContent = "";
}

function renderFailure(message) {
  nodes.summary.dataset.state = "none";
  nodes.headline.textContent = "Status unavailable";
  nodes.meta.textContent = "The status service could not be reached.";
  nodes.componentsPanel.hidden = true;
  nodes.incidentsPanel.hidden = true;
  nodes.error.textContent = `${message} Retrying automatically.`;
  nodes.error.hidden = false;
}

async function load() {
  try {
    const response = await fetch(ENDPOINT, {
      headers: { accept: "application/json" },
      cache: "no-cache",
    });
    if (response.status === 404) {
      renderUnpublished();
      return;
    }
    if (!response.ok) {
      renderFailure(`The status endpoint answered ${response.status}.`);
      return;
    }
    const page = await response.json();
    renderPublished(page);
    nodes.footerUpdated.textContent = `Updated ${formatTime(page.updated_at)}`;
  } catch {
    renderFailure("The status endpoint could not be reached.");
  }
}

let timer = null;

function schedule() {
  if (timer) clearInterval(timer);
  timer = setInterval(() => {
    if (document.visibilityState === "visible") load();
  }, REFRESH_MS);
}

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") load();
});

load();
schedule();
