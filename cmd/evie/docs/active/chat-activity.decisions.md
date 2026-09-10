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
