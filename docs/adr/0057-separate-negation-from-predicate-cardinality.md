# Separate negation from Predicate cardinality

Claims use explicit affirmed or denied polarity, and denial applies only to the
same subject, Predicate, object, scope context, and overlapping Valid Time.
Predicates separately declare expected cardinality of one or many plus their
allowed object kind or type. Cardinality produces deterministic conflict
diagnostics rather than a uniqueness constraint, preserving contradictory
accepted evidence instead of silently rejecting or replacing it. Stage 3 also
computes exact opposite-polarity and overlapping single-cardinality warnings
without persisting a Graph Link or changing any lifecycle.
