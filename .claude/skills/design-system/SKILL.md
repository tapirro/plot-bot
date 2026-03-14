---
name: design-system
description: "Mantissa Design System — CSS tokens, Tailwind config, component patterns, dashboard creation rules. Load when creating or editing HTML/UI."
user-invocable: false
allowed-tools: Read, Glob
---

# Mantissa Design System (MANDATORY for all HTML/UI)

**Every HTML file, dashboard, artifact, or UI output MUST use the Mantissa Design System.**

- **UI folder:** `tools/ui/` — shared CSS, JS, and dashboard HTML files
- **Base CSS:** `tools/ui/base.css` — CSS variables (light/dark) + custom components (source of truth)
- **Tailwind config:** `tools/ui/theme.js` — shared Tailwind theme configuration
- **Template:** `tools/ui/skeleton.html` — clean starting point for new dashboards
- **Reference dashboard:** `tools/ui/observatory.html` — full prototype with all components
- **Spec:** `work/topics/2026-03-08_design-system/DESIGN_SYSTEM.md` — tokens, components, patterns
- **Strategy:** `work/topics/2026-03-08_design-system/STRATEGY.md` — technical decisions

## Quick Rules

1. **Stack:** Tailwind CSS CDN + Google Fonts (Newsreader + Inter + JetBrains Mono) + CSS variables for theming
2. **Serif headings, sans body.** Non-negotiable. `font-serif` for all `<h1>`–`<h3>`, `font-sans` for body.
3. **Warm backgrounds.** `bg-page` (#FCFCF4 light / #141413 dark). Never pure white.
4. **Color = data only.** Teal scale for data viz. UI chrome is grayscale (`text-ink`, `text-muted`, `text-faint`).
5. **Light + dark theme.** CSS variables (`--c-page`, `--c-ink`, etc.) switch via `.dark` class on `<html>`. HTML stays the same.
6. **Heatmap text:** `text-coal` (hardcoded #191918) on light teal cells, `text-white` on dark teal. Never `text-ink` on colored backgrounds.
7. **No shadows, no thick borders.** Cards use `bg-card rounded-card` only. Separators are `border-subtle` hairlines.

## Before Building Any HTML

1. Copy `tools/ui/skeleton.html` as starting point
2. Or add these includes to `<head>`: `<link rel="stylesheet" href="base.css">`, `<script src="https://cdn.tailwindcss.com?plugins=typography"></script>`, `<script src="theme.js"></script>`
3. Add `<script src="nav.js"></script>` before `</body>`
4. Use component patterns from DESIGN_SYSTEM.md section 6
5. **NEVER** copy CSS variables or Tailwind config inline — always link `base.css` + `theme.js`
6. Save new dashboards to `tools/ui/`
