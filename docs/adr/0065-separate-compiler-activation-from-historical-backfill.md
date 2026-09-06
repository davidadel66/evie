# Separate compiler activation from historical backfill

A Compiler Generation begins new-evidence processing at an explicit captured
frontier, while historical backfill is separately selected by bounded scope and
range. This gives the owner control over historical processing and review load:
excluded history remains outside selection, prior review decisions survive
equivalent suggestions, and generation changes never rewrite accepted memory.
