# Product

## Register

product

## Users

Plant-floor and operations staff at Sima Arome, an Indonesian natural-extracts
manufacturer. Four primary roles, each on a distinct screen in a distinct context:

- **Operators** — register incoming lots and upload QC photos. On the floor, often glancing between a screen and physical material; may be on a tablet near the line.
- **QC supervisors** — approve, reject, or recheck AI-assisted QC results. Focused decision work; needs the evidence (image, AI finding, confidence) and the decision controls in one view.
- **Warehouse staff** — confirm/oversee slot assignment and cold-chain status.
- **Managers** — monitor plant health, throughput, and the audit trail at a glance.

Primarily desktop in a plant/office, with tablet use near the line. Bilingual (Bahasa Indonesia / English).

## Product Purpose

A single auditable system that moves a lot through its full lifecycle —
**intake → AI-assisted QC → human QC approval → warehouse assignment → ready for
production** — with minimal clicks and zero ambiguity about where a lot is or
what happens next. Success: any user can tell a lot's exact state and the next
action at a glance, and a lot advances stage to stage without friction or
guesswork.

## Brand Personality

Trustworthy, efficient, clear. The voice of an instrument you rely on, not a
consumer app. It earns confidence through legibility and predictability:
status is unmistakable, decisions are well-supported, nothing is decorative for
its own sake. Calm under load; never flashy.

## Anti-references

- **Consumer-app playfulness** — bouncy mascots, emoji-led tone, gamified flourishes. This is a system of record.
- **Dense ERP / SAP-style gray spreadsheets** — wall-to-wall tiny gray rows with no hierarchy. Legibility and status clarity beat density.
- **Dashboard "hero-metric theater"** — giant gradient vanity numbers with no decision value. Metrics must drive an action or answer a real question.

## Design Principles

1. **Status is the first-class citizen.** A lot's state and its next action must be unmistakable at a glance, in color, label, and position. Never make a user infer where something is.
2. **Evidence beside the decision.** When a user must judge (QC approve/reject, slot choice), the supporting evidence lives in the same view as the control. No hunting.
3. **Earn trust through legibility.** High contrast, plain bilingual labels, predictable layout. The product is auditable; the UI should feel as accountable as the data behind it.
4. **Minimal clicks, stage to stage.** The happy path through the lifecycle is the shortest path. Automate what is safe to automate; surface the manual fallback clearly when it isn't.
5. **Calm under load.** Real-time updates inform without startling. Motion is functional (a status changed), never ornamental.

## Accessibility & Inclusion

- **WCAG 2.1 AA** baseline: body text ≥4.5:1, large/bold ≥3:1, against actual (often tinted) backgrounds.
- **Status never by color alone** — pair every status color with a label and/or icon (color-blind safe; matters for PASS/REVIEW/FAIL and zone health).
- **Full keyboard support** and visible focus on all interactive controls (modals already use focus traps).
- **Reduced motion** honored for every animation (already implemented for the realtime row-highlight); real-time changes must remain observable without motion.
- **Generous touch targets** for tablet/near-line use.
- **Bilingual (ID/EN)** — copy must fit both languages without overflow.
