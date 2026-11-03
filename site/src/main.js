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

const storedLanguage = localStorage.getItem("skein-language");
const browserLanguages = navigator.languages || [navigator.language];
let language = storedLanguage || (browserLanguages.some((item) => item.toLowerCase().startsWith("ru")) ? "ru" : "en");
let stopped = false;
let epoch = 1;

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
  localStorage.setItem("skein-language", language);
}

function updateDemo() {
  const demo = document.querySelector(".swarm-demo");
  demo.classList.toggle("is-stopped", stopped);
  document.querySelector("#epoch-value").textContent = stopped ? `${epoch - 1} → ${epoch}` : String(epoch);
  const status = document.querySelector("#demo-status");
  status.textContent = messages[language][stopped ? "demo.stopped" : "demo.running"];
  const button = document.querySelector("#stop-demo");
  button.textContent = messages[language][stopped ? "demo.reset" : "demo.stop"];
  button.setAttribute("aria-pressed", String(stopped));
}

document.querySelectorAll("[data-lang]").forEach((button) => {
  button.addEventListener("click", () => {
    language = button.dataset.lang;
    translate();
  });
});

document.querySelector("#stop-demo").addEventListener("click", () => {
  if (!stopped) {
    epoch += 1;
    stopped = true;
  } else {
    stopped = false;
  }
  updateDemo();
});

async function copyText(button, text) {
  let copied = false;
  try {
    await navigator.clipboard.writeText(text);
    copied = true;
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
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
  button.textContent = messages[language][copied ? "quick.copied" : "quick.failed"];
  window.setTimeout(() => {
    delete button.dataset.copied;
    button.textContent = messages[language][button.dataset.copyLabel];
  }, 1600);
}

document.querySelectorAll("[data-copy-target]").forEach((button) => {
  button.addEventListener("click", () => {
    const target = document.querySelector(button.dataset.copyTarget);
    copyText(button, target.textContent.trim());
  });
});

translate();
