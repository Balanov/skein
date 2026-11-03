import "@fontsource/ibm-plex-sans/latin-400.css";
import "@fontsource/ibm-plex-sans/latin-500.css";
import "@fontsource/ibm-plex-sans/latin-600.css";
import "@fontsource/ibm-plex-sans/cyrillic-400.css";
import "@fontsource/ibm-plex-sans/cyrillic-500.css";
import "@fontsource/ibm-plex-sans/cyrillic-600.css";
import "@fontsource/ibm-plex-mono/latin-400.css";
import "@fontsource/ibm-plex-mono/latin-500.css";
import "@fontsource/ibm-plex-mono/cyrillic-400.css";
import "@fontsource/ibm-plex-mono/cyrillic-500.css";
import "./style.css";
import { messages } from "./i18n.js";
import { evidence } from "./results.js";

const storedLanguage = localStorage.getItem("skein-language");
const browserLanguages = navigator.languages || [navigator.language];
let language = storedLanguage || (browserLanguages.some((item) => item.toLowerCase().startsWith("ru")) ? "ru" : "en");
let stopped = false;
let epoch = 1;
let benchmarkShape = "independent";
let benchmarkSize = 1000;
let benchmarkScale = "log";
const candidateOrder = ["skein", "asyncio", "langgraph"];
const mechanismSteps = ["one", "two", "three"];
let mechanismStep = 0;
let stepDirection = 1;

function text(key, replacements = {}) {
  let value = messages[language][key] ?? key;
  for (const [name, replacement] of Object.entries(replacements)) value = value.replaceAll(`{${name}}`, replacement);
  return value;
}

function formatNumber(value, digits = 3) {
  return new Intl.NumberFormat(language === "ru" ? "ru-RU" : "en-US", { maximumFractionDigits: digits }).format(value);
}

function formatMs(value) {
  return `${formatNumber(value, 3)} ${language === "ru" ? "мс" : "ms"}`;
}

function formatRatio(value) {
  return formatNumber(value, value >= 100 ? 1 : 2);
}

function renderMechanismStep(animate = false) {
  const name = mechanismSteps[mechanismStep];
  document.querySelectorAll("[data-step]").forEach((button, index) => {
    const active = index === mechanismStep;
    button.setAttribute("aria-selected", String(active));
    button.tabIndex = active ? 0 : -1;
  });
  document.querySelectorAll("[data-point]").forEach((point, index) => {
    point.classList.toggle("is-active", index === mechanismStep);
    point.classList.toggle("is-complete", index < mechanismStep);
  });
  const panel = document.querySelector("#mechanism-step");
  panel.setAttribute("aria-labelledby", `step-tab-${name}`);
  const copy = document.querySelector("#step-copy");
  copy.querySelector("h3").textContent = text(`mechanism.${name}`);
  copy.querySelector("p").textContent = text(`mechanism.${name}Body`);
  document.querySelector("#step-position").textContent = `0${mechanismStep + 1} / 03`;
  document.querySelector("#step-prev").disabled = mechanismStep === 0;
  document.querySelector("#step-next").disabled = mechanismStep === mechanismSteps.length - 1;
  document.querySelector(".step-packet").style.transform = `translate(${38 + mechanismStep * 132}px, 44px)`;
  if (animate && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    copy.animate([
      { opacity: 0, transform: `translateX(${stepDirection * 12}px)` },
      { opacity: 1, transform: "translateX(0)" },
    ], { duration: 180, easing: "ease-out" });
  }
}

function setMechanismStep(index, animate = true) {
  const next = Math.max(0, Math.min(mechanismSteps.length - 1, index));
  if (next === mechanismStep) return;
  stepDirection = next > mechanismStep ? 1 : -1;
  mechanismStep = next;
  renderMechanismStep(animate);
}

function translate() {
  document.documentElement.lang = language;
  document.querySelectorAll("[data-i18n]").forEach((element) => {
    const key = element.dataset.i18n;
    if (messages[language][key]) element.textContent = messages[language][key];
  });
  document.querySelectorAll("[data-lang]").forEach((button) => {
    const active = button.dataset.lang === language;
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-pressed", String(active));
  });
  document.querySelectorAll("[data-copy-label]").forEach((button) => {
    if (!button.dataset.copied) button.textContent = messages[language][button.dataset.copyLabel];
  });
  updateDemo();
  renderMechanismStep();
  renderBenchmark();
  renderStopEvidence();
  localStorage.setItem("skein-language", language);
}

function updateDemo() {
  document.querySelector(".swarm-demo").classList.toggle("is-stopped", stopped);
  document.querySelector("#epoch-value").textContent = stopped ? `${epoch - 1} → ${epoch}` : String(epoch);
  document.querySelector("#demo-status").textContent = text(stopped ? "demo.stopped" : "demo.running");
  const button = document.querySelector("#stop-demo");
  button.textContent = text(stopped ? "demo.reset" : "demo.stop");
  button.setAttribute("aria-pressed", String(stopped));
  const suffix = stopped ? "stop" : "run";
  for (let index = 1; index <= 3; index++) document.querySelector(`#demo-event-${index}`).textContent = text(`demo.${suffix}${index}`);
}

function candidateName(candidate) {
  if (candidate === "skein") return "Skein";
  if (candidate === "asyncio") return "Python asyncio 3.12.16";
  return "LangGraph 1.2.19";
}

function currentResults() {
  if (benchmarkShape === "process") {
    return evidence.process.results.map((result) => ({
      tasks: result.tasks,
      candidate: result.candidate,
      ...result.summary,
    }));
  }
  return evidence.scheduler[benchmarkShape].results.filter((result) => result.tasks === benchmarkSize);
}

function availableSizes(shape) {
  if (shape === "process") return [];
  return [...new Set(evidence.scheduler[shape].results.map((result) => result.tasks))].sort((a, b) => a - b);
}

function renderSizes() {
  const container = document.querySelector("#benchmark-sizes");
  const sizes = availableSizes(benchmarkShape);
  container.hidden = benchmarkShape === "process";
  if (!sizes.includes(benchmarkSize)) benchmarkSize = sizes.includes(1000) ? 1000 : sizes[0];
  container.replaceChildren(...sizes.map((size) => {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = formatNumber(size, 0);
    button.dataset.size = String(size);
    button.setAttribute("aria-pressed", String(size === benchmarkSize));
    button.addEventListener("click", () => {
      benchmarkSize = size;
      renderBenchmark();
    });
    return button;
  }));
}

function comparisonText(results) {
  const values = Object.fromEntries(results.map((result) => [result.candidate, result.median_ms]));
  if (benchmarkShape === "process") {
    return text("bench.processProof", {
      asyncRatio: formatRatio(values.asyncio / values.skein),
      langRatio: formatRatio(values.langgraph / values.skein),
    });
  }
  const parts = [];
  if (values.asyncio >= values.skein) parts.push(text("bench.asyncLower", { ratio: formatRatio(values.asyncio / values.skein) }));
  else parts.push(text("bench.asyncFaster", { ratio: formatRatio(values.skein / values.asyncio) }));
  if (values.langgraph) parts.push(text("bench.langLower", { ratio: formatRatio(values.langgraph / values.skein) }));
  if (!values.langgraph) parts.push(text("bench.noLanggraph"));
  return parts.join(" ");
}

function renderBenchmark() {
  document.querySelectorAll("[data-shape]").forEach((button) => button.setAttribute("aria-pressed", String(button.dataset.shape === benchmarkShape)));
  renderSizes();
  const results = currentResults().sort((a, b) => candidateOrder.indexOf(a.candidate) - candidateOrder.indexOf(b.candidate));
  const tasks = benchmarkShape === "process" ? evidence.process.tasks_per_run : benchmarkSize;
  document.querySelector("#benchmark-title").textContent = `${formatNumber(tasks, 0)} ${text(`shape.${benchmarkShape}Title`)}`;
  document.querySelector("#benchmark-context").textContent = text(benchmarkShape === "process" ? "bench.cold" : "bench.warm");
  document.querySelector("#scale-state").textContent = text(`bench.scale.${benchmarkScale}`);
  const max = Math.max(...results.map((result) => result.median_ms));
  const chart = document.querySelector("#benchmark-chart");
  chart.replaceChildren(...results.map((result, index) => {
    const row = document.createElement("div");
    row.className = `bar-row ${result.candidate === "skein" ? "is-skein" : ""}`;
    const ratio = benchmarkScale === "linear"
      ? result.median_ms / max
      : Math.log10(result.median_ms / 0.01) / Math.log10(max / 0.01);
    const name = document.createElement("span");
    name.className = "bar-name";
    name.textContent = candidateName(result.candidate);
    const track = document.createElement("div");
    track.className = "bar-track";
    const fill = document.createElement("div");
    fill.className = "bar-fill";
    fill.style.setProperty("--scale", Math.max(0.01, ratio).toFixed(5));
    fill.style.setProperty("--stagger", `${index * 70}ms`);
    track.append(fill);
    const value = document.createElement("strong");
    value.className = "bar-value";
    value.textContent = formatMs(result.median_ms);
    row.append(name, track, value);
    return row;
  }));
  document.querySelector("#benchmark-insight").textContent = comparisonText(results);
  const body = document.querySelector("#benchmark-table-body");
  body.replaceChildren(...results.map((result) => {
    const row = document.createElement("tr");
    if (result.candidate === "skein") row.className = "is-skein";
    const candidate = document.createElement("td");
    candidate.textContent = candidateName(result.candidate);
    const values = [formatMs(result.median_ms), formatMs(result.p95_ms), formatNumber(result.median_tasks_per_second, 0), String(result.runs)];
    row.append(candidate, ...values.map((value) => {
      const cell = document.createElement("td");
      cell.textContent = value;
      return cell;
    }));
    return row;
  }));
  document.querySelector("#benchmark-method").textContent = text(`bench.method.${benchmarkShape === "process" ? "process" : "scheduler"}`);
  const rawName = benchmarkShape === "process" ? evidence.raw_files.process : evidence.raw_files[benchmarkShape];
  document.querySelector("#raw-result-link").href = `./results/${rawName}`;
}

function setupPanels(buttonSelector, prefix, copySelector) {
  document.querySelectorAll(buttonSelector).forEach((button) => {
    button.addEventListener("click", () => {
      const name = button.dataset[button.hasAttribute("data-mechanism-target") ? "mechanismTarget" : "proofTarget"];
      document.querySelectorAll(buttonSelector).forEach((item) => item.setAttribute("aria-pressed", String(item === button)));
      document.querySelectorAll(`[id^="${prefix}-"]`).forEach((panel) => {
        if (panel.classList.contains("code-panel")) panel.hidden = panel.id !== `${prefix}-${name}`;
      });
      const target = document.querySelector(`#${prefix}-${name} pre`);
      document.querySelector(copySelector).dataset.copyTarget = `#${target.id}`;
    });
  });
}

async function copyText(button, value) {
  let copied = false;
  try {
    await navigator.clipboard.writeText(value);
    copied = true;
  } catch {
    const area = document.createElement("textarea");
    area.value = value;
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.append(area);
    area.select();
    try {
      copied = document.execCommand("copy");
    } catch {
      copied = false;
    } finally {
      area.remove();
    }
  }
  button.dataset.copied = String(copied);
  button.textContent = text(copied ? "quick.copied" : "quick.failed");
  window.setTimeout(() => {
    delete button.dataset.copied;
    button.textContent = text(button.dataset.copyLabel);
  }, 1600);
}

document.querySelectorAll("[data-lang]").forEach((button) => button.addEventListener("click", () => {
  language = button.dataset.lang;
  translate();
}));

document.querySelector("#stop-demo").addEventListener("click", () => {
  if (!stopped) {
    epoch += 1;
    stopped = true;
  } else {
    stopped = false;
  }
  updateDemo();
});

document.querySelectorAll("[data-shape]").forEach((button) => button.addEventListener("click", () => {
  benchmarkShape = button.dataset.shape;
  renderBenchmark();
}));

document.querySelector("#scale-toggle").addEventListener("click", () => {
  benchmarkScale = benchmarkScale === "log" ? "linear" : "log";
  renderBenchmark();
});

document.querySelectorAll("[data-step]").forEach((button, index) => {
  button.addEventListener("click", () => setMechanismStep(index));
  button.addEventListener("keydown", (event) => {
    let next;
    if (event.key === "ArrowRight") next = (mechanismStep + 1) % mechanismSteps.length;
    if (event.key === "ArrowLeft") next = (mechanismStep + mechanismSteps.length - 1) % mechanismSteps.length;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = mechanismSteps.length - 1;
    if (next === undefined) return;
    event.preventDefault();
    setMechanismStep(next);
    document.querySelector(`[data-step="${mechanismSteps[next]}"]`).focus();
  });
});
document.querySelector("#step-prev").addEventListener("click", () => setMechanismStep(mechanismStep - 1));
document.querySelector("#step-next").addEventListener("click", () => setMechanismStep(mechanismStep + 1));
let swipeStart = null;
const stepPanel = document.querySelector("#mechanism-step");
stepPanel.addEventListener("pointerdown", (event) => {
  if (event.isPrimary) swipeStart = event.clientX;
});
stepPanel.addEventListener("pointerup", (event) => {
  if (swipeStart === null) return;
  const distance = event.clientX - swipeStart;
  swipeStart = null;
  if (Math.abs(distance) >= 44) setMechanismStep(mechanismStep + (distance < 0 ? 1 : -1));
});
stepPanel.addEventListener("pointercancel", () => { swipeStart = null; });

document.querySelectorAll("[data-copy-target]").forEach((button) => button.addEventListener("click", () => {
  const target = document.querySelector(button.dataset.copyTarget);
  copyText(button, target.textContent.trim());
}));

setupPanels("[data-mechanism-target]", "mechanism", "#mechanism-copy");
setupPanels("[data-proof-target]", "proof", "#proof-copy");

function renderStopEvidence() {
  const summary = evidence.stop.summary;
  document.querySelector("#stop-median").textContent = formatMs(summary.median_ms);
  document.querySelector("#stop-p95").textContent = formatMs(summary.p95_ms);
  document.querySelector("#stop-max").textContent = formatMs(summary.max_ms);
  document.querySelector("#stop-verified").textContent = `${formatNumber(summary.child_heartbeats_verified_frozen, 0)} / ${formatNumber(summary.trees_stopped, 0)}`;
}
const multiPass = evidence.functional.scenarios.find((scenario) => scenario.name === "multi_pass_data_flow");
document.querySelector("#proof-events-code").textContent = multiPass.events.map((event) => JSON.stringify(event)).join("\n");

async function loadGithubStats() {
  try {
    const response = await fetch("https://api.github.com/repos/Balanov/skein", { headers: { Accept: "application/vnd.github+json" } });
    if (!response.ok) return;
    const repository = await response.json();
    document.querySelector("#gh-stars").textContent = formatNumber(repository.stargazers_count, 0);
    document.querySelector("#gh-forks").textContent = formatNumber(repository.forks_count, 0);
  } catch {
    // Static counts remain visible when GitHub is unavailable.
  }
}

translate();
loadGithubStats();
