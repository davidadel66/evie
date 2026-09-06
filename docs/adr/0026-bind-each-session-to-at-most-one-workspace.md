# Bind each session to at most one Workspace

Each session belongs to either one Workspace or none, and that binding remains
fixed for the session's lifetime. A Workspace supplies the session's default
Agent Preset, although the owner may choose another compatible preset without
changing the Workspace or gaining access to another Workspace's memory and
connections. Multi-Workspace sessions are deferred to avoid implicit data
mixing.
