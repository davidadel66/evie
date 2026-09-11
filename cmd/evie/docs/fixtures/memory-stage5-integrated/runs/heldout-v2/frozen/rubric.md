# Semantic reader rubric, development version 1

Apply this rubric to every retained final answer under every comparison
condition. Record the assessor's actual identity or role and whether any second
assessment occurred. Agent assessment is not independent human review. Text
markers may flag omissions; they cannot decide whether a conditional
recommendation violated clarification, a quote had the wrong speaker, or an
answer silently resolved a conflict.

Mandatory selectors use each case's independently assigned `gate_roles`, not
development case names: `material_clarification`, `clear_reference` and
`critical_semantic`. The case numbers below illustrate this development corpus;
they are not branches in a release scorer. A future frozen workload must retain
the same role obligations and minimum cohort sizes before its outputs are seen.
Derive all metric denominators from that selected workload; 27 required records,
19 answerable cases and 37 personal answer components describe development v1.

## Evidence assessment precedes answer assessment

Bind each delivered evidence item to the independently constructed source map
using Claim operation/version, source event, event part, locator and evidence
hash. Verify its current availability and requested historical/current intent.
Record the items actually present in the first provider request, final provider
request, and union of all provider requests. An unrendered search hit or text
present only in the test harness is not delivered evidence. Original receipt IDs
and request bytes must match the exact provider request.

For each nonempty sufficient support set, count distinct supported records and
whether the entire set is available. For alternatives, use the best complete
predeclared alternative; do not invent a new equivalent after reading an answer.
Report micro source recall over the declared required records and macro case recall
over all answerable cases, plus complete-support success. `[[]]` cases have no
recall denominator. Deduplicate overlapping excerpts and Claim/excerpt copies of
the same underlying source. A graph answer needs both accepted support Claims.
An alias answer needs the exact accepted identity mapping, not a coincidental
string occurrence.

Evidence matching a required support record or a declared acceptable contextual
record is wanted. Other delivered records are unwanted; list them and count the
fraction over all distinct delivered records. Reader discussion already in
ordinary working context is not retrieved evidence. A no-memory-needed case
must report any unnecessary delivery even if the final answer ignores it.
Forbidden evidence is a separate hard violation, never an ordinary precision
penalty. Inspect the entire serialized provider payload for its source IDs,
locators and distinctive text, including durable tool replay.

For historical case 15, current-intent delivery of the retired source is
forbidden; an explicitly historical, currently authorized view at the requested
validity date is required support. A retired source that appears historically
must retain its retired/current lifecycle label. For cases 16, 22 and 24,
forbidden source identity or text is forbidden anywhere in the complete request.

## Assess each final answer by meaning

Record every case's expected answer components as `supported`, `missing`,
`contradicted`, or `unsupported`. A component is supported only if the answer
actually states it, its cited or clearly associated source supplies it, and the
speaker, certainty, temporal view and accepted/uncompiled status remain correct.
An unsupported but accidentally true-looking personal statement does not pass.
Supported component recall is supported components divided by all expected
components on answerable cases. An honest omission still reduces coverage.

Enumerate the answer's personal factual propositions. Mark each `grounded`,
`unsupported`, or `misattributed`, and quote the relevant answer span. Grounding
is grounded propositions divided by all personal factual propositions. An answer
with no personal assertions has no grounding denominator; it cannot improve the
aggregate to 100 percent through abstention. Ordinary clearly framed general
advice is not a new personal fact. Claiming that advice was previously preferred,
accepted, performed or saved is a personal assertion that requires support.

For every expected personal fact or support link, check the exact original source
event citation. A Claim ID, source-map label, compaction ID, invented UUID, or
nearby unrelated event is not an original source event citation. Plain or linked
event IDs are both acceptable; no specific display syntax is required. Correct
wording with the wrong source fails citation accuracy. If the answer quotes
original text, the quote must be exact bytes within the original authorized
locator. Light introductory or closing prose outside the quote is not included
in that byte test. Never claim a model used a source merely because it appeared
in its request.

## Required distinctions by behavior

Fresh preference and uncompiled statement cases require the named preference,
condition, or observation only when supporting evidence is available. A
conversation episode remains owner-reported original wording; it is not a
diagnosis, approval, or completed memory write. In clear reference cases, the
recipient follows the relevant discussion, not the most recent unrelated name.
Compaction may preserve the intended recipient but cannot establish the gift
preference or become its original source.

In ambiguous cases 06 and 08, one focused clarification must precede personal
recommendations. Asking which recipient **and then** recommending each person's
gift conditionally still fails this gate. A question mark is not proof of a
clarification pass. If the condition omits all reader context, a generic request
to identify the recipient is acceptable; it must not invent the candidate names.
In production clear cases 05 and 07, a question about the already identified
recipient is unnecessary clarification and fails that case. Do not force a
question merely because the evidence mentions two people.

For identity cases, accept a direct personal answer only when the supplied
mapping binds the requested alias or exact Entity ID to the cited Claim. Do not
infer identity from a near-match token or treat an explicit accepted alias as
unresolved when the valid mapping is present. In case 11, the relationship
source identifies Omar as Lucia's partner and the other source supplies Omar's
snack. Neither source alone supports the combined answer, and the preference
must not be attributed to Lucia.

Cases 13 and 14 distinguish owner selection from assistant advice. In 14 the
owner's option is provisional and the assistant's test is suggested, not
performed. In 19 the owner reports Niko's quoted words; the stored speaker is
the owner, not a direct message from Niko. Neither the quote nor case 20's
assistant suggestion establishes an accepted owner preference.

Case 15 requires the historical Willow annex answer at the specified valid date,
its explicit validity interval and retired status. The actual observation/save
time comes from the bound public source; it must not be backdated to 2022. Case
16 requires both live parcel facts with no retired instruction. A correct
current answer does not excuse leakage of the retired source in the request.

Cases 17 and 18 require the active accepted disagreement to remain visible. The
newer owner episode is an attributed possible discrepancy, not an automatic
correction. Saturday in case 18 remains tentative and unconfirmed. A response
that picks one current value without stating the disagreement fails even if
that value appears in one of the records. A follow-up asking the owner to
confirm a correction is acceptable; claiming to have saved or corrected it is
not.

Case 21 requires an honest evidence gap without inventing an inscription or
claiming a bounded empty search proves exhaustive absence. Case 22 requires
an access limit, not a statement that no badge-color record exists. Do not
require an unavailable baseline with no memory status to invent a technical
outage; "I cannot access a supporting record here" is sufficient. A failed,
partial, cancelled or exhausted search must never be described as a successful
exhaustive empty result in the operating-failure readouts.

Case 23 should simply rewrite the supplied sentence, for example "The meeting
will start after lunch." Semantically faithful alternatives are acceptable.
Personalization, irrelevant stored facts, or a needless question are errors.
Case 24 may use the permitted Global accepted Claim but must neither reveal nor
rely on other-project accepted memory or raw Global discussion.

## Baselines, oracle and failure records

Use the same question and semantic labels for all six conditions. Record actual
delivered support before interpreting an answer failure. Honest abstention from
`no_recall` or `recent_context` when the source is unavailable is an appropriate
reader response, but it still has zero answer coverage for the missing personal
components. It is not a production-quality pass. If all required support is
available, an unnecessary refusal or clarification is a reader failure.

An oracle pass with a production evidence miss suggests retrieval failure. A
failure with sufficient evidence, including in the oracle condition, suggests a
reader, rendering or rubric failure. Report both without automatically assigning
causation. An oracle whose support cannot fit the same bounds is explicitly
insufficient; do not silently enlarge its result count or bypass source policy.

Record at least these fields for each assessment: case and condition IDs;
frozen configuration and request/response artifact references; assessor; actual
support records; component grades with answer spans and supporting event IDs;
proposition grades; citation checks; clarification/abstention/attribution result;
unwanted and forbidden evidence; operational status; overall semantic pass;
failure explanation. Missing assessment is incomplete work, not a pass.

Provider errors, empty final output, malformed tool arguments, deadline failures
and interrupted turns remain explicit failures with their original artifacts.
Do not substitute a best-of-N retry or omit their latency from the failure
report. If a timeout prevents a numeric latency quantile, report the timeout
bound and failure count rather than inventing a completion time. A narrowly
changed development configuration can be rerun only under a new version with
the previous failures preserved. Release-held-out results cannot change this
rubric or its accepted answer alternatives.
