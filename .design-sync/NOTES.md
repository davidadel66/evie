# design-sync notes

- The UI is an app (`internal/web/ui`, package name `ui`, private, no
  library build). The sync entry is a hand-written barrel,
  `src/designSystem.ts`, that re-exports only props-driven components; the
  data-fetching containers (Spending, CashFlow, Workspaces, WorkspaceWorkflows,
  Usage, Database, DataHub, SpendingAccounts, SpendingTransactions) are left
  out on purpose because they call `/api/...` on mount.
- Styling is Tailwind v4 with `@theme` tokens in `src/theme.css`; JSX carries
  utility classes, so the only complete stylesheet is the compiled one from
  `npm run build` (`dist/assets/index-<hash>.css`). `buildCmd` copies it to
  `dist/design-system.css`, which `cssEntry` points at.
- Fonts are IBM Plex Sans/Mono and Caveat from Google Fonts (`@import url`
  in theme.css), so the render check reports `[FONT_REMOTE]`.
- `src/ui/Icon.tsx` exports individual icon components (Wrench,
  ChevronDown...), not an `Icon` component; they are not synced.

## Known render warns

- `[TOKENS_MISSING] --pane-width, --sdm-tbg, --tw, --streamdown-caret`: set by
  runtime code or by third-party widgets (streamdown's markdown renderer, the
  inspector pane's resize handle), not by the design tokens. Harmless for the
  synced components.
- `[FONT_REMOTE]` IBM Plex Sans/Mono: Google Fonts `@import` in theme.css;
  loads at runtime by design.

## Preview authoring (folded from the 2026-10-08 first-sync waves)

- Every preview wraps its JSX in an `AppFrame` (`bg-app text-ink p-4
  text-[13px]`, the root classes App.tsx sets): the card page is white and
  Evie's text tokens are for the dark surface.
- Preview-only Tailwind arbitrary classes (`h-[210px]`) are not in the
  bundle CSS (Tailwind v4 emits only classes found in app source); frame
  previews with inline `style`.
- Native `<dialog>` components (Dialog, SettingsDialog, CreateWorkspaceDialog)
  call `showModal()` and paint in the top layer, so they stack in a grid:
  `cardMode: single` with a 900x700 viewport.
- Tall or wide views need bigger column viewports: SpendingView 1100x1800,
  CashFlowView 1100x1000 (chart has `min-w-[960px]`), UsageView and
  DataHubView 900x1400; the workflow lists are wide and short, so `column`.
- DataHubView, DatabaseView and UsageView are `flex-1 min-h-0` panes: give
  them a fixed-height flex column (about 640px) or the table drawer collapses.
- Components that fetch on mount are previewed with a guarded `window.fetch`
  wrapper inside the preview module (CreateWorkspaceDialog answers
  `/api/presets/list`; SpendingView answers the two `/api/data/spending/*`
  calls of the containers it embeds). Each card is its own page, so the
  wrapper never leaks. A source change lifting those fetches out would make
  the wrappers unnecessary.
- Internal-state views (TextSizeMenu open, SettingsDialog archived view,
  expanded `<details>`) are opened by a post-mount effect in the preview;
  stable across captures.
- `toLocaleString()` timestamps follow the capture machine's locale and
  zone; never grade on the clock.
- Skipped states, by design: hover, drag, keyboard-only, and anything
  reachable only by submitting (saving, conflict, restoring).

## Re-sync risks

- `dist/types` and `dist/design-system.css` are build outputs: run
  `buildCmd` before the converter or the `.d.ts` contracts and the
  stylesheet go stale.
- The fetch wrappers in SpendingView.tsx and CreateWorkspaceDialog.tsx
  mirror API shapes in `src/api/*.ts`; an API change rots them silently.
- Fixture data in previews mirrors real Evie workflows (finance-daily,
  finance-review) and September 2026 finances; renaming a workflow does
  not break a card but makes it fiction.
- Node 24 and the npm lockfile were used; Google Fonts must be reachable
  for the render check to show IBM Plex.
