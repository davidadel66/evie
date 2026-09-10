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
