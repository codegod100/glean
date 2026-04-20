# Glean Design System

Adapted from the Starbucks-inspired green palette. Warm, confident, and grounded in a four-tier green system with a dark/light theme switch.

## 1. Visual Theme & Atmosphere

Glean is a **warm, focused reading environment**. The canvas alternates between a deep forest green (dark mode) and a warm cream (light mode), with Green Accent (`#00754A`) anchoring all CTAs, links, and brand moments. The palette is deliberately not blue, not purple — it references natural, grounded tones.

Typography uses **Inter** (Google Fonts) as the universal typeface, with tight `-0.01em` letter-spacing across the entire product. A single typeface, a single voice.

Surfaces breathe through rounded geometry: pill buttons (`9999px`), `12px` card corners, and `50%` circular avatars. Shadows are whisper-soft dual-layers, never heavy. The system feels like a well-lit reading room.

**Color-block rhythm (landing page):** Cream/forest hero → white card sections → House Green (`#1E3932`) feature band with white text → cream utility zone → House Green footer.

Logo is a stylized bee: body with horizontal stripes (like text lines on a page), semi-transparent wings that evoke open book pages, round eyes, curved antennae, and a small smile. The bee represents gleaning (collecting nectar/knowledge), social behavior (hives/communities), and reading (the striped body reads like lines of text, wings like turning pages).

## 2. Color Palette

### Primary Greens

| Name         | Hex       | Role                                             |
| ------------ | --------- | ------------------------------------------------ |
| Green Accent | `#00754A` | CTAs, active states, link hovers, brand accent   |
| Green Dark   | `#006241` | Headings on landing page, stronger brand moments |
| House Green  | `#1E3932` | Dark bands, footer, feature sections             |
| Green Uplift | `#2b5148` | Decorative accents, mid-dark green               |
| Green Light  | `#d4e9e2` | Light green utility surfaces, valid-state tints  |

### Dark Theme (default)

| Token              | Value                    | Use                           |
| ------------------ | ------------------------ | ----------------------------- |
| `--spot-bg`        | `#0f1f1a`                | Page background, sidebar      |
| `--spot-surface`   | `#152b24`                | Card background               |
| `--spot-hover`     | `#1a362e`                | Hover state, input background |
| `--spot-text`      | `#ffffff`                | Primary text                  |
| `--spot-secondary` | `rgba(255,255,255,0.70)` | Secondary/metadata text       |
| `--spot-body`      | `rgba(255,255,255,0.87)` | Body copy, article content    |
| `--spot-muted`     | `rgba(255,255,255,0.25)` | Disabled/tertiary text        |
| `--spot-divider`   | `rgba(255,255,255,0.08)` | Borders, dividers             |
| `--spot-outline`   | `rgba(255,255,255,0.20)` | Button borders, input borders |

### Light Theme

| Token              | Value              | Use                           |
| ------------------ | ------------------ | ----------------------------- |
| `--spot-bg`        | `#f2f0eb`          | Page canvas (warm cream)      |
| `--spot-surface`   | `#ffffff`          | Card background               |
| `--spot-hover`     | `#edebe9`          | Hover state (ceramic)         |
| `--spot-text`      | `rgba(0,0,0,0.87)` | Primary text (warm black)     |
| `--spot-secondary` | `rgba(0,0,0,0.58)` | Secondary/metadata text       |
| `--spot-body`      | `rgba(0,0,0,0.70)` | Body copy                     |
| `--spot-muted`     | `rgba(0,0,0,0.25)` | Disabled/tertiary text        |
| `--spot-divider`   | `rgba(0,0,0,0.08)` | Borders, dividers             |
| `--spot-outline`   | `rgba(0,0,0,0.15)` | Button borders, input borders |

### Semantic

| Name   | Hex       | Use               |
| ------ | --------- | ----------------- |
| Red    | `#c82014` | Errors, likes     |
| Orange | `#ffa42b` | Ratings, warnings |
| Blue   | `#539df5` | External links    |

## 3. Typography

**Font:** Inter (Google Fonts), weights 400/500/600/700

**Global:** `letter-spacing: -0.01em` on body

| Role          | Size | Weight | Tailwind Class                                |
| ------------- | ---- | ------ | --------------------------------------------- |
| Page title    | 24px | 700    | `text-2xl font-bold`                          |
| Section title | 18px | 600    | `text-lg font-semibold`                       |
| Body          | 14px | 400    | `text-sm`                                     |
| Small/meta    | 12px | 400    | `text-xs`                                     |
| Button label  | 14px | 700    | `text-sm font-bold uppercase tracking-button` |
| Micro         | 10px | 400    | `text-[10px]`                                 |

## 4. Components

### Buttons

All buttons use full-pill radius (`rounded-pill` = `9999px`).

**Primary Filled:**

```
bg-spot-green text-white rounded-pill px-5 py-2 text-sm font-bold uppercase tracking-button hover:brightness-110 transition
```

**Primary Outlined:**

```
border border-spot-outline text-spot-text rounded-pill px-4 py-1.5 text-xs font-bold uppercase tracking-button hover:border-spot-text transition
```

### Cards

`rounded-xl` (12px) radius, `shadow-spot` elevation, `bg-spot-surface` background.

```
bg-spot-surface rounded-xl p-4 shadow-spot hover:bg-spot-hover-50 transition
```

### Shadows

| Token               | Value                                                    | Use          |
| ------------------- | -------------------------------------------------------- | ------------ |
| `shadow-spot`       | `0 0 0.5px rgba(0,0,0,0.14), 0 1px 1px rgba(0,0,0,0.24)` | Cards        |
| `shadow-spot-heavy` | `0 0 6px rgba(0,0,0,0.24), 0 8px 12px rgba(0,0,0,0.14)`  | Modals, hero |

### Navigation

- **Sidebar** (desktop): Fixed left, `w-60`, logo at top, nav links, user profile at bottom
- **Bottom nav** (mobile): Fixed bottom, 5-tab horizontal bar
- **Active link**: `bg-spot-hover text-spot-text font-bold`

### Forms

- Input fields: `bg-spot-hover rounded-pill px-5 py-2 text-sm focus:ring-2 focus:ring-spot-green`
- Textareas: `bg-spot-hover rounded-lg px-4 py-2 text-sm focus:ring-2 focus:ring-spot-green`
- File inputs: native browser style with pill-styled file button

### Badges / Tags

- Unread count: `bg-spot-green/20 text-spot-green px-2.5 py-0.5 rounded-full font-bold`
- Category pills: `bg-spot-hover text-spot-secondary px-4 py-1.5 rounded-full font-bold`
- Active category: `bg-spot-active-pill-bg text-spot-active-pill-text`

## 5. Layout

- **Content max-width:** `max-w-6xl` (72rem)
- **Sidebar:** `w-60` fixed left (desktop only)
- **Main content:** `lg:ml-60` offset with `px-4 lg:px-8 py-6`
- **Landing page:** Full-width (`w-full`) — no max-width wrapper
- **Footer:** `bg-spot-surface border-t border-spot-divider`

### Responsive Breakpoints

| Name    | Width      | Nav behavior                    |
| ------- | ---------- | ------------------------------- |
| Mobile  | < 768px    | Bottom tab nav, stacked layouts |
| Tablet  | 768–1023px | Bottom nav, wider gutters       |
| Desktop | 1024px+    | Sidebar nav, 3-column grids     |

## 6. Tailwind Config

All custom colors live under the `spot` namespace in `tailwind.config.js`. CSS variables provide theme switching via `[data-theme]` attribute. Build output goes to `static/output.css` via `npx tailwindcss`.

## 7. Asset Pipeline

- **CSS build:** `make css` (minified) or `make css-watch` (dev with live reload)
- **Source:** `static/input.css` — contains `@tailwind` directives, CSS variables for themes, `@layer components` for article-body styles, and base utilities
- **Output:** `static/output.css` (gitignored, rebuilt on deploy)
- **Favicon:** `static/favicon.svg` — wheat stalk SVG on House Green background
- **Logo:** Inline SVG in base.html sidebar and index.html hero

## 8. Page Structure

| Page            | Layout                 | Key Features                                   |
| --------------- | ---------------------- | ---------------------------------------------- |
| Index (landing) | Full-width, no sidebar | Hero with mockup, feature cols, dark band, CTA |
| Login           | Centered card          | Bluesky + Atmosphere sign-in buttons           |
| Dashboard       | 2/3 + 1/3 grid         | Articles + trending/recommendations sidebar    |
| Articles        | Full-width list        | Keyboard nav (j/k/o/m), mark-all-read          |
| Article Detail  | `max-w-3xl` centered   | Content, like/share/read buttons, annotations  |
| Feeds           | 2/3 + 1/3 grid         | Feed list with categories + add/import sidebar, refresh button |
| Trending        | Full-width list        | Like/annotation counts on each article         |
| Discover        | Mixed grid             | Recommendations + people + browse all          |
| Annotations     | Full-width list        | Filter by article URL, load more               |
| Profile         | `max-w-2xl` centered   | Avatar, stats, feeds, annotations              |
