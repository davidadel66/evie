# Fail invalid presets without conflating Account Connections

A missing required plugin or Capability makes an Agent Preset invalid and
session creation fails with a repairable diagnostic rather than silently
falling back to `standard`. Missing optional contributions remain visible
warnings. A missing Account Connection does not invalidate the preset or make
the plugin unhealthy; the session may start and guide the owner through
connection setup.
