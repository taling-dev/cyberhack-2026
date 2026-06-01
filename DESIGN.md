---
name: SimaOps AI
description: Auditable QC and operations control surface for a natural-extracts plant
colors:
  ink: "#020617"
  body: "#334155"
  muted: "#64748b"
  faint: "#94a3b8"
  border: "#e2e8f0"
  surface: "#ffffff"
  surface-sunken: "#f8fafc"
  surface-raised: "#f1f5f9"
  primary: "#2563eb"
  primary-soft-bg: "#dbeafe"
  primary-soft-ink: "#1d4ed8"
  pass: "#047857"
  pass-bg: "#d1fae5"
  pass-dot: "#10b981"
  review: "#b45309"
  review-bg: "#fef3c7"
  fail: "#b91c1c"
  fail-bg: "#fef2f2"
  fail-border: "#fecaca"
  warehouse: "#7e22ce"
  warehouse-bg: "#f3e8ff"
typography:
  display:
    fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "normal"
  title:
    fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 700
    lineHeight: 1.3
  eyebrow:
    fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 700
    letterSpacing: "normal"
    textTransform: "uppercase"
  body:
    fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 500
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace"
    fontSize: "0.75rem"
    fontWeight: 400
rounded:
  sm: "4px"
  md: "8px"
  lg: "12px"
  full: "9999px"
spacing:
  xs: "8px"
  sm: "12px"
  md: "16px"
  lg: "20px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.surface}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
  card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.lg}"
    padding: "16px"
  status-pass:
    backgroundColor: "{colors.pass-bg}"
    textColor: "{colors.pass}"
    rounded: "{rounded.md}"
  status-review:
    backgroundColor: "{colors.review-bg}"
    textColor: "{colors.review}"
    rounded: "{rounded.md}"
  status-fail:
    backgroundColor: "{colors.fail-bg}"
    textColor: "{colors.fail}"
    rounded: "{rounded.md}"
---

# Design System: SimaOps AI

## 1. Overview

**Creative North Star: "The Instrument Panel"**

SimaOps is read the way an operator reads a gauge: state first, at a glance, with
zero ambiguity. The surface is a calm, near-white slate field that recedes so the
information on it carries the meaning. Hierarchy comes from weight and color, not
from chrome; a lot's status is always the loudest thing in view, and the control
that acts on it sits right beside the evidence. The system favors legibility over
density and predictability over novelty: every screen across intake, QC, warehouse,
and dispatch shares one card vocabulary, one neutral ramp, and one status color
language, so a user who learns one screen has learned them all.

It explicitly rejects three things. **Consumer-app playfulness** (no mascots,
emoji-led tone, gamified flourishes) because this is a system of record.
**Dense ERP/SAP gray spreadsheets** because legibility and status clarity beat
wall-to-wall rows. **Dashboard hero-metric theater** because a number that doesn't
drive an action doesn't earn its size.

**Key Characteristics:**
- Slate neutral field; color reserved for status and the single primary action.
- Status is never decorative and never color-only: color + label + (often) icon.
- Cards at rest are flat with a hairline border; one soft shadow, never both loud.
- Monospace for machine values (lot IDs, temperatures); sans for everything human.
- Bilingual (ID/EN); copy fits both without overflow.

## 2. Colors

A slate-neutral foundation carrying a single blue primary and a four-role status palette (pass / review / fail / warehouse). Color density is low by design: most of any screen is neutral.

### Primary
- **Signal Blue** (#2563eb): The one interactive accent. Primary buttons, links (#2563eb / `blue-600`), active nav, focus rings (#dbeafe / `blue-100`). Soft pairing #dbeafe bg / #1d4ed8 ink for assignable chips.

### Secondary
- **Warehouse Violet** (#7e22ce): Reserved for warehouse/slot-assignment affordances (#f3e8ff bg / #7e22ce ink). Domain marker, not a second brand color.

### Tertiary (Status)
- **Pass Green** (#047857 ink / #d1fae5 bg / #10b981 dot): QC PASS, READY_FOR_PRODUCTION, HEALTHY cold-chain zones.
- **Review Amber** (#b45309 ink / #fef3c7 bg): QC REVIEW, WARNING zone health, override prompts.
- **Fail Red** (#b91c1c ink / #fef2f2 bg / #fecaca border): QC FAIL/REJECTED, CRITICAL cold-chain alerts, destructive actions, errors.

### Neutral
- **Ink** (#020617 / `slate-950`): Primary text, values, headings.
- **Body** (#334155 / `slate-700`): Secondary text, table cells.
- **Muted** (#64748b / `slate-500`): Captions, sublabels, metadata.
- **Faint** (#94a3b8 / `slate-400`): Empty states, disabled, placeholder icons.
- **Border** (#e2e8f0 / `slate-200`): All hairline borders and dividers.
- **Surface** (#ffffff): Cards and primary panels.
- **Sunken** (#f8fafc / `slate-50`): Page background, table headers, zone tiles.
- **Raised** (#f1f5f9 / `slate-100`): Inline chips, skeleton fills, badges.

### Named Rules
**The Status-Loudest Rule.** On any screen, the loudest colored element is the lot/zone status. Color is spent on status and the single primary action first; if a decorative element competes with a status badge for attention, the decoration is wrong.

**The Never-Color-Alone Rule.** Every status color is paired with a text label and, where space allows, an icon or dot. PASS/REVIEW/FAIL and zone health must be distinguishable without color (color-blind safe, WCAG-aligned).

## 3. Typography

**Display / Body Font:** System UI stack (`system-ui, -apple-system, Segoe UI, Roboto, sans-serif`) — one family, weight-driven hierarchy.
**Label/Mono Font:** System monospace (`ui-monospace, SFMono-Regular, Menlo, monospace`).

**Character:** Deliberately neutral and native. No brand display face; trust comes from legibility, not personality in the letterforms. The mono face is functional, it marks machine-generated values so the eye separates a lot ID or a temperature from prose instantly.

### Hierarchy
- **Display** (700, 1.5rem/24px, lh 1.2): Page titles ("QC Review: LOT-…").
- **Title** (700, 1.125rem/18px): Card and modal headings.
- **Eyebrow** (700, 0.8125rem/13px, UPPERCASE, tracking-normal): Section headers inside cards ("STORAGE ZONES", "ASSIGNMENT QUEUE").
- **Body** (400, 0.875rem/14px, lh 1.5): Table cells, descriptions. Cap prose at 65–75ch.
- **Label** (500, 0.75rem/12px): Sublabels, captions, badge text.
- **Mono** (400, 0.75rem/12px): Lot numbers, IDs, temperatures, timestamps.

### Named Rules
**The Mono-For-Machines Rule.** Anything machine-generated or precise (lot IDs, °C readings, timestamps, request IDs) is set in mono. Human language is never mono.

**The Tracking-Normal Rule.** Uppercase eyebrows use normal tracking, not wide letter-spacing. The all-caps + weight already signals "section"; added tracking reads as 2023 kicker styling, which is banned.

## 4. Elevation

Near-flat. Surfaces rest as white cards on a sunken slate-50 page, separated by a 1px #e2e8f0 border. Depth is communicated by the border + background-tier step (sunken → surface → raised), not by stacked shadows. A single soft `shadow-sm` sits under cards for the faintest lift; `shadow-xl` is reserved for modals/overlays only.

### Shadow Vocabulary
- **Resting card** (`box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05)` / `shadow-sm`): Default lift under cards and the assignment table.
- **Overlay** (`shadow-xl`): Modals and the assignment dialog only.

### Named Rules
**The Border-Or-Shadow, Not-Both-Loud Rule.** A card carries a 1px slate-200 border plus at most a soft `shadow-sm`. Never pair a visible border with a heavy (≥16px blur) drop shadow; that ghost-card look is forbidden.

## 5. Components

### Buttons
- **Shape:** Gently curved (8px / `rounded-md`); status/filter pills may be full-pill.
- **Primary:** Signal Blue (#2563eb) bg, white text, 8–12px padding, `font-semibold`. Hover darkens to #1d4ed8.
- **Domain action:** Warehouse actions use violet (#7e22ce) by the same shape rules.
- **Hover / Focus:** Background shift on hover; visible focus ring (#dbeafe) on keyboard focus. All interactive controls keyboard-reachable.

### Chips / Badges
- **Status badge:** Pill or `rounded-md`, tinted bg + matching ink from the status palette, label text always present; PASS adds a #10b981 dot. Never color-only.
- **Count badge:** Soft tone bg (e.g. #f3e8ff / #7e22ce for "N pending").

### Cards / Containers
- **Corner:** 12px (`rounded-lg`) for cards/sections; 8px for inner tiles.
- **Background:** White surface on slate-50 page; inner tiles use slate-50.
- **Shadow:** `shadow-sm` only (see Elevation).
- **Border:** 1px #e2e8f0.
- **Internal padding:** 16px (`md`); compact tiles 12px.

### Inputs / Fields
- **Style:** 1px slate border, white bg, `rounded-md`.
- **Focus:** Blue border shift + ring; never remove the focus indicator.
- **Error:** Fail-red text/border; error text in #b91c1c.

### Navigation
- **Style:** Persistent sidebar; role-gated items. Active item carries the Signal Blue marker; default slate-700, hover slate-950.

### Signature Component: Status Tile
The per-zone cold-chain tile and the QC recommendation badge are the signature pattern: a bordered tile whose **entire background tints to the status tone** (emerald/amber/red), with the health score or recommendation as mono/label inside. This is "The Instrument Panel" made literal, the tile *is* the gauge reading.

## 6. Do's and Don'ts

### Do:
- **Do** make a lot's status the loudest element on screen (color + label + icon).
- **Do** place the supporting evidence (QC image, AI finding, confidence) in the same view as the decision control.
- **Do** keep color to status + the single blue primary; let slate carry the rest.
- **Do** set lot IDs, temperatures, and timestamps in mono.
- **Do** pair every status color with a text label so it survives color blindness and grayscale.
- **Do** verify body text hits ≥4.5:1; bump muted slate toward ink if it's close on a tinted tile.
- **Do** give every animation a `prefers-reduced-motion` path (the realtime row-highlight already does).

### Don't:
- **Don't** signal status by color alone, ever.
- **Don't** add consumer-app playfulness: mascots, emoji-led tone, gamified flourishes.
- **Don't** build dense ERP/SAP gray spreadsheets: tiny gray rows with no hierarchy.
- **Don't** ship hero-metric theater: giant gradient vanity numbers with no decision value.
- **Don't** pair a visible border with a heavy drop shadow (no ghost cards); pick one.
- **Don't** put wide tracking on uppercase eyebrows, and don't add a kicker above every section.
- **Don't** over-round: cards top out at 12px; full-pill is only for tags/buttons.
- **Don't** use gradient text or `background-clip: text` for emphasis; use weight or size.
