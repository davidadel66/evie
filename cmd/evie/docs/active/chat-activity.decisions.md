# Chat activity decisions

- David explicitly approved replacing the existing tool-card presentation.
  This supersedes the tool/thought display choices in the serve documents;
  all acceptance, persistence, cancellation and approval boundaries remain.
- Keep the flat transcript reducer and derive grouped presentation from it.
  Stable turn IDs on historical items allow partial pages to join on prepend.
- Add optional agent presentation callbacks at accepted-user and committed-
  assistant boundaries. Existing Events consumers retain their callbacks.
  The web callback projects public text only; no private continuation state.
- Committed public parts are authoritative. A tool-free committed assistant
  establishes turn success. Explicit commentary remains progress; legacy text
  is classified using whether that assistant requests tools. Provisional live
  text stays in activity until the commit supplies its role.
- Tool intent means requested. Exact command execution is not inferred by
  parsing shell strings. Successful bash outcomes say 'Ran command'; failed
  or declined outcomes never imply the intended edit/read succeeded.
- Durations use accepted-user to terminal timestamps. Failed live turns show
  an incomplete label; successful durations replay from persisted timestamps.
  Public commentary is model output, so its frequency cannot be guaranteed.

- File inspection uses the content recorded for the selected tool action.
  It does not read current disk bytes or execute source code. Full edit
  previews exist in live approval state; historical replacements remain
  explicitly partial instead of being reconstructed from unrelated reads.
- Selection stores the session ID and tool item key, deriving the current
  preview on each render. This pins the selected action while preserving live
  approval/outcome updates and prevents a different session reusing its key.
- File action rows open the inspector instead of inline disclosures. The
  inspector owns code, side-by-side before/after changes, and exact tool
  details. Pending file approvals retain their controls in chat; their
  full previews open only in the inspector. This reflects David’s follow-up
  request to remove the duplicate inline diffs.
- David's later file-view refinement removes the Details tab and routine status
  block. Read files show only breadcrumb path and highlighted source. Edit
  comparisons remain available; partial/proposed/error qualifiers stay concise,
  and recorded approval state is accessible in the path tooltip.
- David's September 11 request moves generic tool arguments and results from
  chat to the selected action's inspector. Expanded chat retains compact,
  readable action rows. The inspector shows useful action and outcome details
  first; exact arguments and results remain inert, unchanged strings behind
  **Debug details**, closed by default. Existing file presentation and approval
  previews remain unchanged.
- Generic tool selection follows the file selection boundary: session ID and
  item key identify the action, and the inspector derives its current approval
  and result state from the transcript. Later actions do not replace selection;
  session or workbench navigation clears it. Keyboard opening, closing, and
  focus restoration apply to generic tool inspection as well as files.
- Both open and collapsed activity show the latest routine memory receipt while retaining
  earlier warnings, interrupted requests, historical or retired evidence, and
  conflicts. Source inspection preserves access to the original receipts and
  request sequence under Debug details. This is presentation grouping only;
  evidence identity, status, scope, attribution, and current access do not change.

- David's follow-up requires readable memory results, not just tool metadata.
  Known recorded memory envelopes are decoded only for display; the raw strings
  stay unchanged. Values, polarity, scope, recorded status, paging and search
  outcomes appear before Debug details in both the tool inspector and a separate
  same-turn results area in memory inspection.
- Tool result history and retrieval receipts are distinct. The selected
  session/snapshot identifies the turn whose tool results are shown, while the
  source-inspection API still controls original evidence and current access.
  Empty retrieval does not imply an empty direct listing. Neither result
  rendering nor request association asserts that an answer cited a source.
