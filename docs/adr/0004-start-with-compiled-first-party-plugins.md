# Start with compiled first-party plugins

Evie's first plugin system will register first-party Go modules compiled into
the Evie executable, with configuration controlling which plugins are enabled.
It will not initially load executable packages into the running process or
promise a third-party compatibility interface. If independent plugin
distribution becomes a real need, Evie can add a versioned external-process
protocol later. This establishes the composition and lifecycle model without
taking on executable-package trust, crash isolation, and compatibility before
they are required. The Plugin Manager may start and stop an already-compiled
plugin at runtime; adding or changing plugin code still requires rebuilding and
restarting Evie.
