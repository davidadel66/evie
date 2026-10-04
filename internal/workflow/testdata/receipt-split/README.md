# Receipt split

Read a receipt's total from its text with an AI step, ask the owner to
confirm it when the reading is low-confidence, and split the total evenly
between people in whole cents. The tip is split separately by a
deterministic step; the AI step sees only the receipt text it declares.
The run key is the receipt ID, so one receipt is split once.

This is a test example for AI steps and durable interruptions (Stage 14
sub-stage 7).
