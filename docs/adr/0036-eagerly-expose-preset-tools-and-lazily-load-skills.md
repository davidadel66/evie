# Eagerly expose preset tools and lazily load skills

Initially, each model request receives the schemas for every tool in the
session's fixed Agent Preset. The model receives a compact catalog of skill
names and summaries and loads full reviewed skill instructions only when
needed. Presets remain intentionally small; tool groups or an on-demand catalog
may later reduce schema cost without changing the session's pinned composition
or authority.
