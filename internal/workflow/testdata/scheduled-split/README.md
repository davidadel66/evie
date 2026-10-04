# Scheduled split

A test example for Workflow Schedules (Stage 14 sub-stage 11): every day at
07:00 in New York it splits 1000 cents between three people. The slot's
local date and time are its inputs `day` and `at` and form its Run Key, so
each slot is one logical run, and the schedule's Catch-up Policy
(`each_missed`, at most 3 runs) decides what runs after downtime.
