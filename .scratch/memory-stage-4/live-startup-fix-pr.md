When the local database has no Workspaces or projects, `/api/context-sessions/list` encoded nil slices as JSON null. The web chooser called `.map` on those fields and the entire page went blank. Return empty arrays for absent Workspaces, Projects and Sessions so a fresh or partially populated database can render the chooser.

This includes only the existing two-file startup fix and its HTTP regression, discovered while smoke-testing the Stage 4 server deployment. Other local drafts remain excluded. Populated lists, active scope, selection, persistence and authorization behavior are unchanged.

Validation:
- `go test ./internal/web -run '^TestContextSessionHTTPEncodesEmptyCollectionsAsArrays$' -count=1`: failed before the fix with all three null collections; passed afterward.
- `./scripts/verify-change.sh`: passed on the isolated combined source.
- Independent Standards and Spec reviews: passed.
- Actual browser reproduced the crash; replacing only the null response lists with empty arrays made the same page render. The installed fixed binary subsequently passed the actual-browser smoke check against the unmodified API: chooser and review inbox render, global candidate listing returns 200 with an empty array, and no page errors occur.

Existing Icon.tsx fast-refresh and Vite bundle-size warnings remain. The unchanged dependency lockfile also reports the existing nanoid advisory noted in #152.
