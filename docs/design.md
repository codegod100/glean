# Glean Design System

Brutalist / minimal. Monochrome base (ink + paper) with a single green accent. Monospace typography, sharp corners, thick 2px borders, hard offset shadows. Built with SvelteKit + Tailwind CSS v4.

## 1. Visual Theme & Atmosphere

Glean is a **raw, structural reading environment**. The canvas is stark: near-black ink on near-white paper (light mode), inverted in dark mode. A single accent green carries the brand and all interactive highlights. No soft shadows, no rounded pills, no gradients — just borders, type, and contrast.

**Typography** uses **JetBrains Mono** (Google Fonts) as the universal typeface, weights 400–800. Labels and metadata are uppercase with wide tracking. The aesthetic is terminal-like: dense, precise, unornamented.

**Geometry** is sharp — every radius is `0px`. Borders are 2px solid. Shadows are hard offsets (`4px 4px 0 0`), never blurred — they read as physical depth, a hallmark of brutalist UI. Interactive elements translate on hover/press (`translate(-1px,-1px)` → deeper shadow → `translate(1px,1px)` → flush), giving a tactile, mechanical feel.

**Layout** is a single centered column (`max-w-5xl`) under a sticky top header bar. No sidebar. The header carries the wordmark, inline nav, search, and a user dropdown. Footer is a structured multi-column band at the bottom of every page.

**Logo** is the favicon glyph (the stylized bee mark) rendered as a boxed letter "G" — a square accent-green tile with the letter, paired with a heavy uppercase "Glean" wordmark.

## 2. Color Palette

The system uses CSS custom properties (defined in `web/src/app.css`) that swap via `[data-theme]`. All components reference these tokens (e.g. `var(--accent)`), never hardcoded hex.

### Light Theme (default)

| Token          | Value     | Use                           |
| -------------- | --------- | ----------------------------- |
| `--bg`         | `#fafaf7` | Page canvas (warm paper)      |
| `--fg`         | `#0a0a0a` | Primary text, borders (ink)   |
| `--surface`    | `#f0efe9` | Card / panel background       |
| `--border`     | `#0a0a0a` | All borders (2px solid)       |
| `--muted`      | `#6b6b6b` | Secondary / metadata text     |
| `--faint`      | `#c8c8c2` | Disabled, tertiary fills      |
| `--accent`     | `#00754a` | Links, active states, brand   |
| `--accent-ink` | `#ecfff4` | Accent-tinted surfaces        |
| `--danger`     | `#c82014` | Destructive actions, sign-out |

### Dark Theme

| Token          | Value     | Use                           |
| -------------- | --------- | ----------------------------- |
| `--bg`         | `#0a0a0a` | Page canvas (ink)             |
| `--fg`         | `#f5f5ef` | Primary text, borders (paper) |
| `--surface`    | `#161616` | Card / panel background       |
| `--border`     | `#f5f5ef` | All borders (inverted)        |
| `--muted`      | `#9a9a9a` | Secondary / metadata text     |
| `--faint`      | `#3a3a3a` | Disabled, tertiary fills      |
| `--accent`     | `#00754a` | Links, active states, brand   |
| `--accent-ink` | `#062018` | Accent-tinted surfaces        |
| `--danger`     | `#ff5a4d` | Destructive actions           |

> The accent green (`#00754a`) is identical in both light and dark themes.

## 3. Typography

**Font:** JetBrains Mono (Google Fonts), weights 400 / 500 / 600 / 700 / 800.

| Role            | Class                                                                  |
| --------------- | ---------------------------------------------------------------------- |
| Page title      | `text-2xl font-extrabold uppercase tracking-tight`                     |
| Section heading | `text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]` |
| Body            | `text-sm` (base), `text-[var(--muted)]` (secondary)                    |
| Button label    | `text-[0.72rem] font-bold uppercase` (via `btn`)                       |
| Tag / chip      | `text-[0.68rem] font-semibold uppercase` (via `chip` / `tag`)          |
| Micro/meta      | `text-[0.7rem] text-[var(--muted)]`                                    |

## 4. Components

All interactive primitives are defined as Tailwind v4 `@utility` classes in `app.css`.

### Buttons

| Class            | Style                                                      | Use                          |
| ---------------- | ---------------------------------------------------------- | ---------------------------- |
| `btn`            | 2px border, hard offset shadow, uppercase, press animation | Default buttons              |
| `btn btn-accent` | `btn` + accent green background, white text                | Primary CTAs (Sign in, Save) |
| `btn btn-ghost`  | `btn` with no border/shadow until hover                    | Toolbar / icon buttons       |

Hover: `translate(-1px,-1px)` + deeper shadow. Active: `translate(1px,1px)` + flush.

### Chips & Tags

- **`chip`** — small uppercase pill with 1.5px border. Add `data-active="true"` for the inverted (fg/bg) active state. Used for filters, like/read toggles, counts.
- **`tag`** — even smaller uppercase label with 1px border, `--surface` background. Used for annotation tags, categories.

### Panels & Cards

- **`panel`** — `--surface` background, 2px `--border` border. The base card container.
- **`panel-press`** — adds the hover/press translate + hard shadow animation. Used on interactive cards (articles, trending, profiles).

### Forms

- **`input-brutal`** — 2px border, monospace, focus pushes `translate(-1px,-1px)` with a hard shadow.
- Textareas and selects reuse `input-brutal`.

### Shadows

| Token              | Value                       | Use                  |
| ------------------ | --------------------------- | -------------------- |
| `--shadow-hard-sm` | `2px 2px 0 0 var(--border)` | Buttons, small cards |
| `--shadow-hard`    | `4px 4px 0 0 var(--border)` | Hover lift, modals   |

Modal dialogs use inline `shadow-[6px_6px_0_0_var(--border)]`.

### Navigation

- **Header bar** (all breakpoints): sticky top, 2px bottom border, `max-w-5xl`. Wordmark left, inline nav (desktop), search/shortcuts/user menu right.
- **Mobile nav row**: below the header, horizontal-scroll row of nav chips (hidden on `md+`).
- **Active nav item**: inverted `bg-[var(--fg)] text-[var(--bg)]`.
- **User menu**: dropdown panel with hard shadow (Profile, Install, Sign out).

### Article body

Rendered RSS/HTML content uses the `article-body` utility: monospace base, uppercase headings, 2px borders on `pre`/`img`/`iframe`/`table`, accent-green links and blockquote borders, accent-green `<mark>`.

## 5. Layout

| Element         | Spec                                   |
| --------------- | -------------------------------------- |
| Content width   | `max-w-5xl` (64rem), `px-4`            |
| Header height   | `h-14` (3.5rem) sticky                 |
| Content padding | `py-8`                                 |
| Footer          | `border-t-2`, multi-column, full-width |

### Responsive Behavior

| Breakpoint | Nav                     | Layout                        |
| ---------- | ----------------------- | ----------------------------- |
| `< 768px`  | Mobile nav row (scroll) | Single column, stacked grids  |
| `≥ 768px`  | Inline header nav       | Multi-column grids where used |

## 6. Tailwind / CSS Pipeline

- **Tailwind CSS v4** via `@tailwindcss/vite` (no `tailwind.config.js`).
- **Source:** `web/src/app.css` — `@import "tailwindcss"`, `@theme` block for design tokens, `:root` / `[data-theme]` for theme variables, `@utility` blocks for component classes.
- **Build:** `cd web && bun run build` (Vite + SvelteKit). Output is the SvelteKit adapter-node build in `web/build/`.
- No separate CSS build step — Vite compiles Tailwind on the fly.

## 7. Asset Pipeline

- **Static assets:** `web/static/` — `favicon.svg` (bee logo), `manifest.json`, PNG icons, `banner.png`. Served at the root (`/favicon.svg`, etc.).
- **Logo component:** `web/src/lib/components/Logo.svelte` — boxed "G" tile + wordmark, sizes `sm` / `md` / `lg`.
- **Icons:** `web/src/lib/components/Icon.svelte` — monochrome line-icon set (stroke=currentColor, square caps), referenced by `name`.

## 8. Page Structure

| Page            | Layout                         | Key Features                                                 |
| --------------- | ------------------------------ | ------------------------------------------------------------ |
| Index (landing) | Full-width sections, no chrome | Hero + mock dashboard panel, feature grid, accent band, CTA  |
| Login           | Centered, chromeless           | Handle input + actor typeahead, OAuth start, register        |
| Dashboard       | Single column                  | Counts, unread articles, lazy recs/digest/trending/people    |
| Articles        | Single column list             | Search, status/category chips, sort, expanded scroll-to-read |
| Article Detail  | Single column, centered prose  | Like/read/share, fetch-content, text-select annotate popover |
| Feeds           | 2/3 list + 1/3 sidebar         | Categories, add/edit/remove, OPML import/export, refresh     |
| Trending        | Single column list             | Scope toggle (All / For me), sign-in prompt                  |
| Library         | Two columns                    | Liked articles + annotations, independent pagination         |
| Profile         | Centered                       | Header + stats, settings (digest/expanded/languages), feeds  |
| Stats           | Single column                  | Metric categories as panels with monospace values            |
