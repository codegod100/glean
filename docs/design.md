# Design System Inspired by Spotify

## 1. Visual Theme & Atmosphere

Spotify's web interface is a dark, immersive music player that wraps listeners in a near-black cocoon (`#121212`, `#181818`, `#1f1f1f`) where album art and content become the primary source of color. The design philosophy is "content-first darkness" — the UI recedes into shadow so that music, podcasts, and playlists can glow. Every surface is a shade of charcoal, creating a theater-like environment where the only true color comes from the brand Accent Purple (`#a855f7`) and the album artwork itself.

The typography uses SpotifyMixUI and SpotifyMixUITitle — proprietary fonts from the CircularSp family (Circular by Lineto, customized for Spotify) with an extensive fallback stack that includes Arabic, Hebrew, Cyrillic, Greek, Devanagari, and CJK fonts, reflecting Spotify's global reach. The type system is compact and functional: 700 (bold) for emphasis and navigation, 600 (semibold) for secondary emphasis, and 400 (regular) for body. Buttons use uppercase with positive letter-spacing (1.4px–2px) for a systematic, label-like quality.

What distinguishes Spotify is its pill-and-circle geometry. Primary buttons use 500px–9999px radius (full pill), circular play buttons use 50% radius, and search inputs are 500px pills. Combined with heavy shadows (`rgba(0,0,0,0.5) 0px 8px 24px`) on elevated elements and a unique inset border-shadow combo (`rgb(18,18,18) 0px 1px 0px, rgb(124,124,124) 0px 0px 0px 1px inset`), the result is an interface that feels like a premium audio device — tactile, rounded, and built for touch.

**Key Characteristics:**

- Near-black immersive dark theme (`#121212`–`#1f1f1f`) — UI disappears behind content
- Accent Purple (`#a855f7`) as singular brand accent — never decorative, always functional
- SpotifyMixUI/CircularSp font family with global script support
- Pill buttons (500px–9999px) and circular controls (50%) — rounded, touch-optimized
- Uppercase button labels with wide letter-spacing (1.4px–2px)
- Heavy shadows on elevated elements (`rgba(0,0,0,0.5) 0px 8px 24px`)
- Semantic colors: negative red (`#f3727f`), warning orange (`#ffa42b`), announcement blue (`#539df5`)
- Album art as the primary color source — the UI is achromatic by design
- Light/dark theme toggle — persisted in localStorage, dark is default

## 2. Color Palette & Roles

All theme-dependent colors use CSS custom properties defined on `:root` (dark, default) and `[data-theme="light"]` (light). Accent and semantic colors are constant across themes.

### Tailwind Token Mapping

The `spot.*` namespace provides semantic tokens. Theme-dependent tokens resolve to CSS variables.

| Token                  | CSS Variable          | Dark (default)                   | Light                            | Use                              |
| ---------------------- | --------------------- | -------------------------------- | -------------------------------- | -------------------------------- |
| `spot.purple`          | —                     | `#a855f7`                        | `#a855f7`                        | Primary accent                   |
| `spot.purple-border`   | —                     | `#9333ea`                        | `#9333ea`                        | Accent border variant            |
| `spot.bg`              | `--spot-bg`           | `#121212`                        | `#f5f5f5`                        | Page background                  |
| `spot.surface`         | `--spot-surface`      | `#181818`                        | `#ffffff`                        | Cards, containers, sidebar       |
| `spot.hover`           | `--spot-hover`        | `#1f1f1f`                        | `#f3f4f6`                        | Interactive surface, input bg    |
| `spot.hover-50`        | `--spot-hover-50`     | `rgba(31,31,31,0.5)`             | `rgba(243,244,246,0.5)`          | Card hover with transparency     |
| `spot.text`            | `--spot-text`         | `#ffffff`                        | `#111827`                        | Primary text, headings           |
| `spot.secondary`       | `--spot-secondary`    | `#b3b3b3`                        | `#6b7280`                        | Secondary text, muted labels     |
| `spot.body`            | `--spot-body`         | `#cbcbcb`                        | `#374151`                        | Article body text                |
| `spot.muted`           | `--spot-muted`        | `#4d4d4d`                        | `#9ca3af`                        | Very muted text, rank numbers    |
| `spot.divider`         | `--spot-divider`      | `rgba(77,77,77,0.2)`             | `#e5e7eb`                        | Subtle border dividers           |
| `spot.divider-30`      | `--spot-divider-30`   | `rgba(77,77,77,0.3)`             | `#d1d5db`                        | Slightly stronger dividers, hr   |
| `spot.outline`         | `--spot-outline`      | `#7c7c7c`                        | `#d1d5db`                        | Visible borders on buttons/inputs|
| `spot.placeholder`     | `--spot-placeholder`  | `#4d4d4d`                        | `#9ca3af`                        | Input placeholder text           |
| `spot.active-pill-bg`  | `--spot-active-bg`    | `#ffffff`                        | `#111827`                        | Active filter pill background    |
| `spot.active-pill-text`| `--spot-active-text`  | `#121212`                        | `#ffffff`                        | Active filter pill text          |
| `spot.shadow`          | `--spot-shadow`       | `rgba(0,0,0,0.3) 0px 8px 8px`    | `rgba(0,0,0,0.08) 0px 2px 8px`  | Card elevation shadow            |
| `spot.shadow-heavy`    | `--spot-shadow-heavy` | `rgba(0,0,0,0.5) 0px 8px 24px`   | `rgba(0,0,0,0.1) 0px 4px 16px`  | Dialog/elevated panel shadow     |
| `spot.red`             | —                     | `#f3727f`                        | `#f3727f`                        | Error states                     |
| `spot.orange`          | —                     | `#ffa42b`                        | `#ffa42b`                        | Warning states                   |
| `spot.blue`            | —                     | `#539df5`                        | `#539df5`                        | Info states                      |

### Shadows

- **Card** (`var(--spot-shadow)`): Cards, dropdowns
- **Heavy** (`var(--spot-shadow-heavy)`): Dialogs, menus, elevated panels
- **Inset Border** (`rgb(18,18,18) 0px 1px 0px, rgb(124,124,124) 0px 0px 0px 1px inset`): Input border-shadow combo (dark only)

## 3. Theme Toggle

The app supports dark (default) and light themes. Theme preference is stored in `localStorage` under the key `theme`.

### Implementation

- CSS custom properties are defined on `:root` for dark and `[data-theme="light"]` for light
- A `<script>` block runs before render to set `data-theme` on `<html>`, preventing flash of wrong theme
- Toggle buttons appear in the sidebar footer (desktop) and mobile top bar
- Icon: moon (when in dark mode) / sun (when in light mode)
- Default: dark

### CSS Variables

```css
:root {
  --spot-bg: #121212;
  --spot-surface: #181818;
  --spot-hover: #1f1f1f;
  --spot-hover-50: rgba(31,31,31,0.5);
  --spot-text: #ffffff;
  --spot-secondary: #b3b3b3;
  --spot-body: #cbcbcb;
  --spot-muted: #4d4d4d;
  --spot-divider: rgba(77,77,77,0.2);
  --spot-divider-30: rgba(77,77,77,0.3);
  --spot-outline: #7c7c7c;
  --spot-placeholder: #4d4d4d;
  --spot-active-bg: #ffffff;
  --spot-active-text: #121212;
  --spot-shadow: rgba(0,0,0,0.3) 0px 8px 8px;
  --spot-shadow-heavy: rgba(0,0,0,0.5) 0px 8px 24px;
}
[data-theme="light"] {
  --spot-bg: #f5f5f5;
  --spot-surface: #ffffff;
  --spot-hover: #f3f4f6;
  --spot-hover-50: rgba(243,244,246,0.5);
  --spot-text: #111827;
  --spot-secondary: #6b7280;
  --spot-body: #374151;
  --spot-muted: #9ca3af;
  --spot-divider: #e5e7eb;
  --spot-divider-30: #d1d5db;
  --spot-outline: #d1d5db;
  --spot-placeholder: #9ca3af;
  --spot-active-bg: #111827;
  --spot-active-text: #ffffff;
  --spot-shadow: rgba(0,0,0,0.08) 0px 2px 8px;
  --spot-shadow-heavy: rgba(0,0,0,0.1) 0px 4px 16px;
}
```

## 4. Typography Rules

### Font Families

- **Title**: `SpotifyMixUITitle`, fallbacks: `CircularSp-Arab, CircularSp-Hebr, CircularSp-Cyrl, CircularSp-Grek, CircularSp-Deva, Helvetica Neue, helvetica, arial, Hiragino Sans, Hiragino Kaku Gothic ProN, Meiryo, MS Gothic`
- **UI / Body**: `SpotifyMixUI`, same fallback stack

### Hierarchy

| Role             | Font              | Size             | Weight  | Line Height  | Letter Spacing | Notes                        |
| ---------------- | ----------------- | ---------------- | ------- | ------------ | -------------- | ---------------------------- |
| Section Title    | SpotifyMixUITitle | 24px (1.50rem)   | 700     | normal       | normal         | Bold title weight            |
| Feature Heading  | SpotifyMixUI      | 18px (1.13rem)   | 600     | 1.30 (tight) | normal         | Semibold section heads       |
| Body Bold        | SpotifyMixUI      | 16px (1.00rem)   | 700     | normal       | normal         | Emphasized text              |
| Body             | SpotifyMixUI      | 16px (1.00rem)   | 400     | normal       | normal         | Standard body                |
| Button Uppercase | SpotifyMixUI      | 14px (0.88rem)   | 600–700 | 1.00 (tight) | 1.4px–2px      | `text-transform: uppercase`  |
| Button           | SpotifyMixUI      | 14px (0.88rem)   | 700     | normal       | 0.14px         | Standard button              |
| Nav Link Bold    | SpotifyMixUI      | 14px (0.88rem)   | 700     | normal       | normal         | Navigation                   |
| Nav Link         | SpotifyMixUI      | 14px (0.88rem)   | 400     | normal       | normal         | Inactive nav                 |
| Caption Bold     | SpotifyMixUI      | 14px (0.88rem)   | 700     | 1.50–1.54    | normal         | Bold metadata                |
| Caption          | SpotifyMixUI      | 14px (0.88rem)   | 400     | normal       | normal         | Metadata                     |
| Small Bold       | SpotifyMixUI      | 12px (0.75rem)   | 700     | 1.50         | normal         | Tags, counts                 |
| Small            | SpotifyMixUI      | 12px (0.75rem)   | 400     | normal       | normal         | Fine print                   |
| Badge            | SpotifyMixUI      | 10.5px (0.66rem) | 600     | 1.33         | normal         | `text-transform: capitalize` |
| Micro            | SpotifyMixUI      | 10px (0.63rem)   | 400     | normal       | normal         | Smallest text                |

### Principles

- **Bold/regular binary**: Most text is either 700 (bold) or 400 (regular), with 600 used sparingly. This creates a clear visual hierarchy through weight contrast rather than size variation.
- **Uppercase buttons as system**: Button labels use uppercase + wide letter-spacing (1.4px–2px), creating a systematic "label" voice distinct from content text.
- **Compact sizing**: The range is 10px–24px — narrower than most systems. Spotify's type is compact and functional, designed for scanning playlists, not reading articles.
- **Global script support**: The extensive fallback stack (Arabic, Hebrew, Cyrillic, Greek, Devanagari, CJK) reflects Spotify's 180+ market reach.

## 5. Component Stylings

### Buttons

**Accent Pill**

- Background: `#a855f7`
- Text: `var(--spot-bg)` (dark in dark mode, light in light mode)
- Padding: 8px 16px
- Radius: 9999px (full pill)
- Use: Primary CTAs, add buttons

**Dark Pill**

- Background: `var(--spot-hover)`
- Text: `var(--spot-text)` or `var(--spot-secondary)`
- Padding: 8px 16px
- Radius: 9999px (full pill)
- Use: Navigation pills, secondary actions

**Outlined Pill**

- Background: transparent
- Text: `var(--spot-text)`
- Border: `1px solid var(--spot-outline)`
- Radius: 9999px
- Use: Follow buttons, secondary actions

**Circular Play**

- Background: `#a855f7`
- Text: `var(--spot-bg)`
- Padding: 12px
- Radius: 50% (circle)
- Use: Play/pause controls

### Cards & Containers

- Background: `var(--spot-surface)`
- Radius: 6px–8px
- No visible borders on most cards
- Hover: `var(--spot-hover-50)` background
- Shadow: `var(--spot-shadow)` on elevated

### Inputs

- Background: `var(--spot-hover)`
- Text: `var(--spot-text)`
- Radius: 500px (pill)
- Focus ring: `#a855f7`
- Placeholder: `var(--spot-placeholder)`

### Navigation

- Sidebar: `var(--spot-bg)` background
- Active items: 14px weight 700, `var(--spot-text)`
- Inactive items: 14px weight 400, `var(--spot-secondary)`
- Circular icon buttons (50% radius)
- Brand logo top-left in purple

## 6. Layout Principles

### Spacing System

- Base unit: 8px
- Scale: 1px, 2px, 3px, 4px, 5px, 6px, 8px, 10px, 12px, 14px, 15px, 16px, 20px

### Grid & Container

- Sidebar (fixed) + main content area
- Grid-based album/playlist cards
- Responsive content area fills remaining space

### Whitespace Philosophy

- **Dark compression**: Spotify packs content densely — playlist grids, track lists, and navigation are all tightly spaced. The dark background provides visual rest between elements without needing large gaps.
- **Content density over breathing room**: This is an app, not a marketing site. Every pixel serves the listening experience.

### Border Radius Scale

- Minimal (2px): Badges, explicit tags
- Subtle (4px): Inputs, small elements
- Standard (6px): Album art containers, cards
- Comfortable (8px): Sections, dialogs
- Medium (10px–20px): Panels, overlay elements
- Large (100px): Large pill buttons
- Pill (500px): Primary buttons, search input
- Full Pill (9999px): Navigation pills, search
- Circle (50%): Play buttons, avatars, icons

## 7. Depth & Elevation

| Level              | Treatment                    | Use                            |
| ------------------ | ---------------------------- | ------------------------------ |
| Base (Level 0)     | `var(--spot-bg)` background  | Deepest layer, page background |
| Surface (Level 1)  | `var(--spot-surface)`        | Cards, sidebar, containers     |
| Elevated (Level 2) | `var(--spot-shadow)`         | Dropdown menus, hover cards    |
| Dialog (Level 3)   | `var(--spot-shadow-heavy)`   | Modals, overlays, menus        |

## 8. Do's and Don'ts

### Do

- Use semantic color tokens (`spot-bg`, `spot-surface`, `spot-text`, etc.) — they adapt to theme
- Apply Accent Purple (`#a855f7`) only for play controls, active states, and primary CTAs
- Use pill shape (500px–9999px) for all buttons — circular (50%) for play controls
- Apply uppercase + wide letter-spacing (1.4px–2px) on button labels
- Keep typography compact (10px–24px range) — this is an app, not a magazine
- Use theme-aware shadows via CSS variables
- Test all components in both dark and light themes

### Don't

- Don't use Accent Purple decoratively or on backgrounds — it's functional only
- Don't hardcode theme-dependent colors — use CSS variable-backed tokens
- Don't skip the pill/circle geometry on buttons — square buttons break the identity
- Don't use hardcoded shadow values — use `shadow-spot` and `shadow-spot-heavy`
- Don't add additional brand colors — purple + achromatic grays is the complete palette
- Don't use `text-white` or `bg-white` directly — use `text-spot-text` and `bg-spot-active-pill-bg`
- Don't expose raw gray borders — use `border-spot-divider` or `border-spot-outline`

## 9. Responsive Behavior

### Breakpoints

| Name          | Width       | Key Changes           |
| ------------- | ----------- | --------------------- |
| Mobile Small  | <425px      | Compact mobile layout |
| Mobile        | 425–576px   | Standard mobile       |
| Tablet        | 576–768px   | 2-column grid         |
| Tablet Large  | 768–896px   | Expanded layout       |
| Desktop Small | 896–1024px  | Sidebar visible       |
| Desktop       | 1024–1280px | Full desktop layout   |
| Large Desktop | >1280px     | Expanded grid         |

### Collapsing Strategy

- Sidebar: full → collapsed → hidden
- Album grid: 5 columns → 3 → 2 → 1
- Search: pill input maintained, width adjusts
- Navigation: sidebar → bottom bar on mobile
- Theme toggle: always accessible in sidebar footer / mobile header

## 10. Agent Prompt Guide

### Quick Color Reference

| Role           | Token                | Value (dark)   |
| -------------- | -------------------- | -------------- |
| Background     | `bg-spot-bg`         | `#121212`      |
| Surface        | `bg-spot-surface`    | `#181818`      |
| Hover          | `bg-spot-hover`      | `#1f1f1f`      |
| Text primary   | `text-spot-text`     | `#ffffff`      |
| Text secondary | `text-spot-secondary`| `#b3b3b3`      |
| Text body      | `text-spot-body`     | `#cbcbcb`      |
| Text muted     | `text-spot-muted`    | `#4d4d4d`      |
| Accent         | `text-spot-purple`   | `#a855f7`      |
| Divider        | `border-spot-divider`| `rgba(...)`    |
| Outline        | `border-spot-outline`| `#7c7c7c`      |
| Error          | `text-spot-red`      | `#f3727f`      |

### Iteration Guide

1. Use semantic tokens (`spot-bg`, `spot-surface`, `spot-text`) — they handle theme switching
2. Accent Purple (`spot-purple`) for functional highlights only (active, CTA)
3. Pill everything — 500px for large, 9999px for small, 50% for circular
4. Uppercase + wide tracking on buttons — the systematic label voice
5. Theme-aware shadows via `shadow-spot` and `shadow-spot-heavy`
6. Never hardcode `text-white` or `bg-white` — use semantic tokens
