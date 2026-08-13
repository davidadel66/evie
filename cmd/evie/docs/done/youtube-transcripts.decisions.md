# youtube-transcripts - decisions

Companion to `youtube-transcripts.spec.md`. This records research that shaped
the spec and will collect implementation amendments and known gaps.

## Decisions before implementation

- **Normalized channels/videos/transcripts, not one table per channel.** A
  channel is data whose mutable name points at a stable YouTube ID. Per-channel
  tables make cross-channel FTS/search and rename handling needlessly hard.
- **Internal integer keys plus nullable external IDs.** The audited legacy
  corpus contains 826 valid transcripts with no recoverable video ID. Integer
  relational keys preserve them without inventing identities; unique
  `youtube_id` fields remain canonical whenever online identity is known.
- **A separate `~/.evie/transcripts/transcripts.db`.** The imported body is
  already about 76 MiB, unlike the small operational state in `evie.db`.
  Separation keeps backups, query registration, and future corpus growth
  explicit.
- **FTS5 now, vectors later if evidence demands them.** Ranked lexical search,
  snippets, and channel filters solve the immediate library use case with the
  SQLite dependency already present. Embeddings would add model/provider,
  chunking, refresh, and cost decisions before they are needed.
- **Two model tools; heavy work remains a CLI concern.** Tool execution is
  synchronous and its result enters the entire future model context. A 5,000
  video crawl does not belong in one turn; a bounded newest-video refresh does.
- **Serial crawling, not a Go worker pool.** Current yt-dlp guidance and the
  unauthenticated endpoint's bot/rate-limit behavior make parallelism a
  reliability regression. Go removes Selenium brittleness; it does not make
  YouTube's upstream quota disappear.
- **Android player response for captions.** Live research on 2026-08-12 found
  the watch page's WEB caption URLs returned HTTP 200 with empty bodies under
  the subtitle PO-token experiment. An unauthenticated ANDROID 20.10.38 player
  request returned usable JSON3 caption URLs. This is intentionally isolated
  and treated as replaceable undocumented protocol, not a permanent API.
- **Legacy import is exact and loss-averse.** The source has 6,185 clean UTF-8
  files, but only 5,359 exact title-to-manifest matches. The remaining 826 stay
  searchable under placeholder channels with NULL video IDs. Fuzzy matching
  would convert uncertainty into confident corruption.
- **Multiple artifacts coexist.** A later manual caption should supersede a
  generated or imported transcript for normal reads without deleting the old
  artifact. FTS joins through a preferred-artifact view to avoid duplicate
  normal search results.
- **Legacy collection provenance belongs to the artifact.** A matched video's
  current channel may be enriched from a folder placeholder to YouTube's real
  channel. `transcripts.legacy_channel_id` preserves where the copied file came
  from across that reassignment.
- **Requested and selected languages are different facts.** A request for `en`
  may select an `en-US` track. Artifacts store the selected code; cache states
  store the requested code, and a satisfying artifact wins over a stale
  terminal state.
- **Transcript text is hostile web content even after caching.** Direct tools
  and `query_db(db="transcripts")` fence it as data, not instructions. This is
  framing, not a complete prompt-injection defense, especially while `bash`
  remains ungated.

## External evidence

- yt-dlp YouTube extractor/base at commit
  `5d6b8c8cd19785c3086ae3a9ec618c45e25eb3bc`: multiple channel renderers and
  continuation envelopes, visitor data, continuation-loop detection, strict
  external IDs, and subtitle PO-token policies.
- youtube-transcript-api at commit
  `72d79711ec4db95262660029b4d63298b0820502`: Android player context, manual
  before generated selection, consent/block/error distinctions, and caption
  URL handling.
- Live probes on 2026-08-12: `dQw4w9WgXcQ` returned working manual and ASR JSON3
  through the Android client; `@BlenderOfficial/videos` returned current
  `lockupViewModel` entries and browse continuations in groups observed as 30
  (page size is not treated as contractual).

## Legacy audit baseline

Immediate collection directories under `/Users/davidboktor/code/scraper`:

```text
BibleProject          329 files, no manifest,       0 exact IDs
BibleProjectPodcast   136 files, no manifest,       0 exact IDs
DesiringGod          4529 files, 4836 manifest,  4529 exact IDs
PracticingTheWay      155 files, empty manifest,     0 exact IDs
R.C.Sproul            802 files, 3200 manifest,    802 exact IDs
TimKeller              67 files,   28 manifest,     28 exact IDs
WesHuff                167 files, no manifest,       0 exact IDs
total                 6185 files,                 5359 exact IDs
```

All files are valid UTF-8, non-empty, LF-only, and have unique exact paths.
ZIPs duplicate extracted data and contain AppleDouble/mojibake entries, so they
are explicitly excluded. Manifest-only rows are a stale crawl queue, not
transcripts, and are not imported.

## Implementation amendments

- The bounded spec review found ten ambiguities before code. The spec now pins
  placeholder-enrichment order, artifact-level legacy provenance,
  requested-versus-selected language keys, refresh preservation of ready
  state, typed terminal outcome behavior, channel-reassignment FTS updates,
  distinct legacy ID counts, re-import identity reconciliation, import summary
  equations, timestamp/word-count/name derivation, and render-only delimiter
  escaping.
- Stage 1's independent import test initially used `Exact Title.txt` and
  `exact title.txt` to prove case-sensitive manifest matching. On macOS's
  default case-insensitive filesystem those are one file, so the second write
  overwrote the first. The fixture now uses distinct filenames with a
  case-mismatched manifest key; the assertion is unchanged, but the filesystem
  can represent the setup.
- The first live video fetch exposed an overbroad block detector: ordinary
  YouTube HTML mentions `recaptcha` in script metadata, so matching the bare
  word rejected a usable watch page. Detection now requires the actual
  `g-recaptcha` challenge marker, with a regression test that allows incidental
  script metadata.
- The same live page mixes strict object calls with `ytcfg.set(key, value)` and
  non-JSON JavaScript object literals. Aborting on the first call made the
  valid later API key unreachable. Config parsing now merges only decodable
  object calls, ignores other shapes, and still requires each caller's actual
  key/context fields before proceeding.
- The adversarial review found that SQLite read-only mode does not make a
  model-supplied query side-effect free: a leading `SELECT` could be followed
  by another statement, including `ATTACH`. Transcript queries now have a
  transcript-only lexical fence requiring exactly one `SELECT`, allowing only
  whitespace after one optional terminal semicolon, and rejecting
  `ATTACH`/`DETACH`; finance and Evie query behavior remains unchanged.
- Caption URL validation now applies on every redirect hop, not only the URL
  returned by the player response. Every caption destination must remain
  HTTPS, credential-free, and on `youtube.com` or a subdomain before the HTTP
  client follows it.
- Only affirmative private/deleted/removed/unavailable playability reasons are
  cacheable `unavailable` outcomes. Unknown `ERROR` and `UNPLAYABLE` reasons
  are format drift so a new upstream condition cannot poison the terminal
  cache.
- Player metadata is treated as untrusted protocol data before persistence:
  channel IDs must have the UC shape, non-empty publish dates must be strict
  `YYYY-MM-DD`, and non-empty owner profile URLs must be credential-free HTTPS
  YouTube URLs. Legacy manifests now call the same strict video-input parser as
  online fetches instead of maintaining a more permissive duplicate parser.
- Transcript query output is built directly from `sql.Rows` under a budget
  that reserves framing and the truncation note inside the 100 KiB ceiling.
  Rendering stops when the row or byte fence is reached rather than retaining
  all rendered cells in a second in-memory row matrix.
- `ytcfg.set` discovery now uses a small JavaScript lexical scanner that skips
  quoted strings, template literals, and line/block comments. This preserves
  mixed real call handling without allowing fake markers in page text to
  override live configuration.
- Recaptcha detection remains structural rather than matching incidental
  metadata: challenge classes, response fields, and recaptcha form/iframe URLs
  are blocked, while an ordinary script string mentioning `recaptcha` remains
  allowed.
- Review regressions now directly prove both remote persistence branches that
  were specified but under-tested: reassignment to an already-existing real
  channel keeps legacy provenance/FTS correct, and repeated refreshes update
  mutable channel/video metadata without duplicate rows.

## Known gaps accepted for V1

The feature shipped 2026-08-13 with the explicit Out of scope boundaries in
the spec. The YouTube web/Innertube protocol remains undocumented and may need
future client-version or renderer updates; failures are deliberately reported
as format drift rather than cached as no-captions outcomes.

The `youtube_transcript` result is working context, not default response
content. A request to get/fetch a transcript should produce a brief readiness
confirmation; Evie must not echo, reflow, or summarize the full text unless
David explicitly asks for that output. The transcript remains available for
follow-up questions and permanently cached in SQLite.

## Verification evidence

- `go test ./...`, `go vet ./...`, and
  `go test -race ./internal/youtube ./internal/tools ./cmd/ytscribe` passed.
- Both `evie` and `ytscribe` built into `~/go/bin`.
- Real import: 6,185 inserted, 5,359 exact video IDs, 826 unmatched, zero
  failures; the second run skipped all 6,185. Source files were untouched.
- Live single-video fetch saved `dQw4w9WgXcQ`; the second fetch reported cache.
- Live `@BlenderOfficial` scrape saved two newest videos; the second scrape
  reported two cached and zero network saves.
- SQLite `integrity_check` returned `ok`, `foreign_key_check` returned no rows,
  and all 6,188 stored transcripts have matching FTS rows.
- Fresh-context code review found nine issues, including three release blockers
  (multi-statement `query_db`, caption redirect SSRF, and false terminal
  caching). All nine were fixed with regression tests; the same reviewer
  approved the corrections.
