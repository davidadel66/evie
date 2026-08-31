# Audit compatible provider substitutions

The original Composition Receipt remains immutable when the exact provider
implementation it names is unavailable. Before resuming with a newer provider,
Evie records a Compatibility Resolution naming the replacement version and the
contract and schema evidence permitting substitution. An unchanged compatible
contract needs no new approval; a changed contract requires a reviewed new
definition or explicit run migration.
