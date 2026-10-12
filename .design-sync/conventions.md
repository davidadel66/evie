# Building with Evie's components

Evie is a dark, text-dense personal-agent app. Every screen sits on one root
surface, and every component here expects to be inside it. Nothing works
unstyled: the look comes from Tailwind v4 utilities whose names are Evie's
role-named tokens, compiled into `styles.css` (its `@import` chain pulls in
`_ds_bundle.css`, the compiled stylesheet). Read `styles.css` before styling
anything.

## 1. Wrap every design in the app root

```jsx
<div className="bg-app text-ink flex h-screen overflow-hidden text-[13px]">
  {/* sidebar, topbar, panes, your screen */}
</div>
```

`bg-app` + `text-ink` are the surface and the default text color; the
13px base is the app's size. Without this wrapper components render dark
text on white and look broken. There is no provider to wrap; the root
classes are the whole setup. Fonts load from Google Fonts at runtime
(IBM Plex Sans for UI, IBM Plex Mono for anything machine-written).

## 2. The styling idiom: role-named Tailwind utilities

Never write a hex. Use these families, by role; each name below exists in
`styles.css` as a class (`bg-`, `text-`, `border-`) on a `--color-*` token:

| Role | Classes |
|---|---|
| Surfaces | `bg-app` (page), `bg-topbar`, `bg-sidebar`, `bg-card` (a card or dialog), `bg-code` (code and JSON blocks), `bg-muted`, `bg-hover` (hover on rows and nav) |
| Hairlines | `border-hair` (default rule), `border-hair-strong` (controls, dialogs), `border-hair-input` (inputs), `border-hair-bubble` |
| Text | `text-ink` (default), `text-body` (prose, labels), `text-body-dim`, `text-muted-text` (secondary), `text-faint` (section labels), `text-fainter` (placeholders) |
| Accent, Evie's teal | `text-teal`, `text-teal-hover`, `bg-teal` (primary button), `bg-teal-deep`, `border-teal`, `border-teal-hair`; focus rings use `focus-visible:ring-teal` |
| Attention, amber | `bg-amber-bg` with `border-amber-hair` and `text-amber-ink` for a warning banner; `text-amber` for a pending state |
| Errors, red | `bg-danger-banner` with `text-danger-ink` for an alert banner; `text-danger` for a failed state or an error line |
| Type | `font-sans` (default), `font-mono` for ids, hashes, amounts, JSON, tool names; `tabular-nums` on every number column |

Controls are small: `rounded-md border border-hair-strong bg-card px-3 py-2
text-xs`, with `focus-visible:ring-2 focus-visible:ring-teal
focus-visible:outline-none`. A pressed filter uses `aria-pressed` plus
`bg-teal-deep text-teal`. Money is a decimal string of cents, shown with
`formatFlow`-style formatting: negative is money out, and inflows get
`text-teal-hover`.

## 3. Where the truth lives

- `styles.css` and `_ds_bundle.css`: every class that exists. If a class is
  not there, do not invent it; pick the nearest role above.
- `_ds_bundle.css`, top of the file: the `--color-*` and `--font-*` token
  values, compiled from the app's `@theme` block (`tokens/` is empty on
  purpose; the values ship inside the stylesheet).
- `components/<group>/<Name>/<Name>.prompt.md` and `<Name>.d.ts`: how to
  compose each component and its exact props. Views named `*View` are
  presentational: you pass `loading`, `problem`, the report or page, and
  the callbacks; they never fetch.

## 4. One idiomatic screen

```jsx
<div className="bg-app text-ink flex h-screen overflow-hidden text-[13px]">
  <main className="min-w-0 flex-1 overflow-y-auto px-6 py-5">
    <h1 className="text-body mb-3 text-sm font-medium">Spending</h1>
    <SpendingTransactionsView
      month="2026-09" status="all" offset={0} loading={false}
      counts={{ all: 133, unclassified: 26, classified: 93, pending: 0, excluded: 14 }}
      page={page} onStatus={setStatus} onPage={setOffset} onRetry={reload}
    />
  </main>
</div>
```

The component brings its own filters, table and paging; the layout glue
around it is yours, in the same utility vocabulary.
