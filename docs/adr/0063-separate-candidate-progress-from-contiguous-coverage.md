# Separate candidate progress from contiguous coverage

Independent Stage 4 jobs may persist unaccepted Memory Candidates out of order,
while Compilation Coverage records exact completed ranges and unresolved gaps
without advancing its contiguous frontier across a gap. This avoids one failed
episode blocking later review without pretending missing evidence was processed;
accepted Semantic Operations retain current revision checks, and extraction
cannot depend on earlier unaccepted candidates.
