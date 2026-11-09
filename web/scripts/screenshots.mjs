#!/usr/bin/env node
// README screenshot harness (Playwright).
//
// Captures the pages in ROUTES, in both themes, into docs/screenshots/. It is
// the committed, curated set a reader sees before running anything — not
// throwaway visual QA. Regenerate it when the UI changes.
//
// Quick start
//   1. Start the app: `make up` (product) or `make dev-up` (dev loop), both
//      serve the UI on http://localhost:5173.
//   2. `make screenshots`
//
// One-time setup:  (cd web && npx playwright install chromium)
//
// Output:          ../docs/screenshots (override with SCREENSHOTS_DIR)
// Theme:           dark and light (the app is dark by default and switches on
//                  prefers-color-scheme: light, which `colorScheme` drives)
//
// Fonts matter: a capture environment with no fonts renders glyphs as nothing,
// so every PNG comes out blank/black. The harness refuses a capture with almost
// no text instead of committing a blank image.
//
// Environment overrides
//   BASE_URL                      base URL to capture (default http://localhost:5173)
//   SCREENSHOTS_DIR               output directory (default ../docs/screenshots)
//   SCREENSHOTS_THEMES            comma list, e.g. "dark" or "dark,light"
//   SCREENSHOTS_MIN_TEXT          minimum visible characters (default 40)
//   SCREENSHOTS_IGNORE_CONSOLE_ERRORS  set to 1 to tolerate console.error output

import { mkdir } from "node:fs/promises";

// ── CONFIG ───────────────────────────────────────────────────────────────────
// Each route:
//   name             output filename stem, e.g. "catalogue"
//   path             URL path appended to BASE_URL
//   ready            CSS selector that must exist before capture (optional but
//                    strongly recommended — it is how you stop racing the render)
//   errorSelector    optional: fail if this element is visible (e.g. "[data-error]")
//   loadingSelector  optional: fail if this element is still visible (e.g. ".skeleton")
//
// The catalogue is public, so it captures without a session. `/files` is
// session-gated and is not in the set: a logged-in capture needs a seeded
// account, which is a separate decision.
const ROUTES = [
  { name: "catalogue", path: "/", ready: "main" },
  { name: "login", path: "/login", ready: "form" },
  { name: "signup", path: "/signup", ready: "form" },
];

const BASE_URL = (process.env.BASE_URL || "http://localhost:5173").replace(/\/+$/, "");
const OUT_DIR = process.env.SCREENSHOTS_DIR || "../docs/screenshots";
const THEMES = (process.env.SCREENSHOTS_THEMES || "dark,light")
  .split(",")
  .map((s) => s.trim())
  .filter(Boolean);
const MIN_TEXT_CHARS = Number(process.env.SCREENSHOTS_MIN_TEXT || 40);
const IGNORE_CONSOLE_ERRORS = process.env.SCREENSHOTS_IGNORE_CONSOLE_ERRORS === "1";
const VIEWPORT = { width: 1440, height: 900 };
// ─────────────────────────────────────────────────────────────────────────────

// The app switches theme with `prefers-color-scheme` only, so the browser
// context's colorScheme is enough. A hook is kept for an app that switches with
// a class or attribute instead.
async function applyTheme(_page, _theme) {}

if (ROUTES.length === 0) {
  console.error("[screenshots] No routes configured.");
  console.error("  Edit ROUTES in web/scripts/screenshots.mjs, then re-run `make screenshots`.");
  process.exit(1);
}

let chromium;
try {
  ({ chromium } = await import("playwright"));
} catch {
  console.error("[screenshots] Playwright is not installed.");
  console.error("  Install it once: (cd web && npm i -D playwright && npx playwright install chromium)");
  process.exit(2);
}

async function isVisible(page, selector) {
  try {
    return await page.isVisible(selector);
  } catch {
    return false;
  }
}

await mkdir(OUT_DIR, { recursive: true });

const browser = await chromium.launch();
const written = [];
const refused = [];

for (const route of ROUTES) {
  for (const theme of THEMES) {
    const file = `${OUT_DIR}/${route.name}-${theme}.png`;
    const url = `${BASE_URL}${route.path}`;
    const problems = [];

    const page = await browser.newPage({ viewport: VIEWPORT, colorScheme: theme });
    page.on("pageerror", (err) => problems.push(`page error: ${err.message}`));
    if (!IGNORE_CONSOLE_ERRORS) {
      page.on("console", (msg) => {
        if (msg.type() === "error") problems.push(`console error: ${msg.text()}`);
      });
    }

    try {
      await page.goto(url, { waitUntil: "networkidle", timeout: 30000 });
      if (route.ready) await page.waitForSelector(route.ready, { timeout: 15000 });
      await applyTheme(page, theme);
      await page.waitForTimeout(300); // let late paint settle
    } catch (err) {
      problems.push(`navigation: ${err.message}`);
    }

    if (problems.length === 0) {
      if (route.errorSelector && (await isVisible(page, route.errorSelector))) {
        problems.push(`error element ${route.errorSelector} is visible`);
      }
      if (route.loadingSelector && (await isVisible(page, route.loadingSelector))) {
        problems.push(`loading element ${route.loadingSelector} is still visible`);
      }
      const text = (await page.evaluate(() => (document.body ? document.body.innerText : ""))).trim();
      if (text.length < MIN_TEXT_CHARS) {
        problems.push(`almost no visible text (${text.length} chars) — blank or crashed page`);
      }
    }

    if (problems.length === 0) {
      await page.screenshot({ path: file });
      written.push(file);
    } else {
      refused.push({ file, theme, url, problems });
    }
    await page.close();
  }
}

await browser.close();

for (const file of written) console.log(`[screenshots] wrote ${file}`);

if (refused.length > 0) {
  console.error(`\n[screenshots] REFUSED to write ${refused.length} capture(s):`);
  for (const item of refused) {
    console.error(`  ${item.file}  (${item.theme}, ${item.url})`);
    for (const problem of item.problems) console.error(`      - ${problem}`);
  }
  console.error("\nFix the page or the route config; a capture that failed a guard is never committed.");
  process.exit(1);
}

if (written.length === 0) {
  console.error("[screenshots] nothing captured.");
  process.exit(1);
}

console.log(`[screenshots] done — ${written.length} image(s) in ${OUT_DIR}/`);
