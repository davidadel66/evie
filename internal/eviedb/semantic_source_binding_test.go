package eviedb

import (
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerSpanBindingRules(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		literal       memory.TypedLiteral
		predicate     string
		span          string // "" means Evie-proposed
	}{
		{"case and punctuation fold", "Long day. Remember: my favourite café is BLUE-BOTTLE! Thanks.", text("blue bottle"), "favorite_cafe", "Remember: my favourite café is BLUE-BOTTLE!"},
		{"whitespace folds", "My parking spot is level 2\n  bay 14 now", text("level 2 bay 14"), "parking_spot", "My parking spot is level 2\n  bay 14 now"},
		{"inflection folds", "I am allergic to peanuts.", text("peanut"), "allergy", "I am allergic to peanuts."},
		{"-ves plural", "Hi. My keepsake is indigo cotton scarves.", text("indigo cotton scarf"), "keepsake", "My keepsake is indigo cotton scarves."},
		{"-ies plural", "We love small cities. Ok.", text("small city"), "preference", "We love small cities."},
		{"-ves plural of -ve", "Pack my gloves. Thanks.", text("glove"), "packing", "Pack my gloves."},
		{"multi-sentence value", "Note this. Rule one! Rule two! Done", text("Rule one! Rule two!"), "rules", "Rule one! Rule two!"},
		{"possessive keeps the name", "My friend Sarah's birthday party is Friday.", text("Sarah"), "friend", "My friend Sarah's birthday party is Friday."},
		{"value in a sentence about someone else", "Selma likes indigo cotton scarves.", text("indigo cotton scarf"), "keepsake", ""},
		{"value must be consecutive words", "I prefer the dark UI mode.", text("dark mode"), "theme", ""},
		{"absent value is Evie-proposed", "Read this article and remember what matters.", text("Lisbon"), "home_city", ""},
		{"thousands separators", "Our budget is $1,500 for the trip.", literal(memory.LiteralInteger, "1500"), "budget", "Our budget is $1,500 for the trip."},
		{"decimal trailing zeros", "The dose is 2.50 mg. Ok?", literal(memory.LiteralDecimal, "2.5"), "dose", "The dose is 2.50 mg."},
		{"number words", "I have two kids and a dog.", literal(memory.LiteralInteger, "2"), "kids", "I have two kids and a dog."},
		{"different number", "I have three kids.", literal(memory.LiteralInteger, "2"), "kids", ""},
		{"leading zeros", "Wake me at 06:30 please.", text("6:30"), "alarm", "Wake me at 06:30 please."},
		{"month-name date", "Busy week. My sister's birthday is June 3rd. Cake?", literal(memory.LiteralDate, "2026-06-03"), "sister_birthday", "My sister's birthday is June 3rd."},
		{"day-first date", "We moved in on the 3rd of June, 2026.", literal(memory.LiteralDate, "2026-06-03"), "move_in", "We moved in on the 3rd of June, 2026."},
		{"numeric date", "Dentist on 6/3.", literal(memory.LiteralDate, "2026-06-03"), "dentist_visit", "Dentist on 6/3."},
		{"ISO date", "Deadline 2026-06-03 firm.", literal(memory.LiteralDate, "2026-06-03"), "deadline", "Deadline 2026-06-03 firm."},
		{"different stated year", "It was June 3, 2025.", literal(memory.LiteralDate, "2026-06-03"), "anniversary", ""},
		{"different day", "Her birthday is June 13.", literal(memory.LiteralDate, "2026-06-03"), "sister_birthday", ""},
		{"relative date stays Evie-proposed", "My dentist visit is tomorrow.", literal(memory.LiteralDate, "2026-10-02"), "dentist_visit", ""},
		{"datetime by calendar date", "The flight leaves Oct 2 in the afternoon.", literal(memory.LiteralDatetime, "2026-10-02T15:00:00Z"), "flight", "The flight leaves Oct 2 in the afternoon."},
		{"boolean by Predicate words", "Lunch was fine. I'm vegetarian these days.", literal(memory.LiteralBoolean, "true"), "is_vegetarian", "I'm vegetarian these days."},
		{"boolean without Predicate words", "Read this article.", literal(memory.LiteralBoolean, "true"), "is_vegetarian", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := bindLiteral(tc.message, tc.literal, tc.predicate, memory.PolarityAffirmed)
			checkBinding(t, tc.message, binding, tc.span)
		})
	}
}

func TestOwnerSpanBindingForEntityNames(t *testing.T) {
	sarah := []string{"Sarah Connor", "Sarah"}
	tennis := []string{"Tennis", "tennis"}
	for _, tc := range []struct {
		name, message string
		names         [][]string
		span          string
	}{
		{"alias in one sentence", "Sarah plays tennis every week.", [][]string{sarah, tennis}, "Sarah plays tennis every week."},
		{"adjacent sentences", "I talked to Sarah today. She plays tennis now.", [][]string{sarah, tennis}, "I talked to Sarah today. She plays tennis now."},
		{"too far apart", "Sarah called. Weather is nice. Lunch was good. Tennis is fun.", [][]string{sarah, tennis}, ""},
		{"name absent", "She plays tennis.", [][]string{sarah, tennis}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokens := bindingTokens(tc.message)
			var needs []bindingOccurrences
			for _, names := range tc.names {
				needs = append(needs, phraseOccurrences(tokens, names...))
			}
			checkBinding(t, tc.message, bindOwnerSource(tc.message, bindingClaim{needs: needs}), tc.span)
		})
	}
}

func TestOwnerSpanBindingNarrowsLongSentences(t *testing.T) {
	message := strings.Repeat("filler words without any stop ", 40) + "my locker code word is pelican " + strings.Repeat("and more trailing words here ", 40)
	binding := bindLiteral(message, text("pelican"), "locker_code_word", memory.PolarityAffirmed)
	if binding.authority != memory.AuthorityOwnerStatement || len(binding.evidence) > 200 || !strings.Contains(binding.evidence, "pelican") {
		t.Fatalf("long sentence was not narrowed around the value: %+v", binding)
	}
	if err := verifyRememberSourceEvidence(sourceFor(binding), message); err != nil {
		t.Fatal(err)
	}
}

func text(value string) memory.TypedLiteral { return literal(memory.LiteralText, value) }

func literal(kind memory.LiteralKind, value string) memory.TypedLiteral {
	return memory.TypedLiteral{Kind: kind, Value: value}
}

func sourceFor(binding ownerSourceBinding) memory.SemanticSource {
	source := memory.SemanticSource{EventPart: memory.EvidenceContent, Actor: memory.SemanticActorOwner,
		SourceType: memory.SourceTypeUserMessage, Eligibility: memory.EligibilityEligible}
	binding.apply(&source)
	return source
}

func checkBinding(t *testing.T, message string, binding ownerSourceBinding, span string) {
	t.Helper()
	if err := verifyRememberSourceEvidence(sourceFor(binding), message); err != nil {
		t.Fatalf("binding does not verify against its message: %+v: %v", binding, err)
	}
	if span == "" {
		if binding.authority != memory.AuthorityEvieProposed || binding.kind != memory.LocatorWhole || binding.evidence != "" || binding.hash != evidenceHash(message) {
			t.Fatalf("want Evie-proposed whole-message reference, got %+v", binding)
		}
		return
	}
	kind := memory.LocatorUTF8ByteRange
	if span == message {
		kind = memory.LocatorWhole
	}
	if binding.authority != memory.AuthorityOwnerStatement || binding.kind != kind || binding.evidence != span || binding.hash != evidenceHash(span) {
		t.Fatalf("want owner span %q, got %+v", span, binding)
	}
}

func TestRememberSourceShapesAndEvidenceVerification(t *testing.T) {
	message := "Noise first. Remember that my favorite color is teal."
	owner := sourceFor(bindLiteral(message, text("teal"), "favorite_color", memory.PolarityAffirmed))
	legacy := memory.SemanticSource{EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorWhole, EvidenceSHA256: evidenceHash(message),
		Actor: memory.SemanticActorOwner, SourceType: memory.SourceTypeUserMessage, Authority: memory.AuthorityOwnerStatement,
		Evidence: message, Eligibility: memory.EligibilityEligible}
	for _, accepted := range []memory.SemanticSource{owner, legacy} {
		if err := verifyRememberSourceEvidence(accepted, message); err != nil {
			t.Fatalf("accepted source shape rejected: %+v: %v", accepted, err)
		}
	}
	rejected := map[string]memory.SemanticSource{}
	widened := owner
	widened.Evidence = message
	rejected["span quoting more than its range"] = widened
	shifted := owner
	shifted.LocatorValue = "0:5"
	rejected["range moved off its quote"] = shifted
	laundered := owner
	laundered.Authority = memory.AuthorityEvieProposed
	rejected["Evie-proposed span"] = laundered
	quoting := sourceFor(bindOwnerSource(message, bindingClaim{}))
	quoting.Evidence = message
	rejected["Evie-proposed quoting the message"] = quoting
	claimed := sourceFor(bindOwnerSource(message, bindingClaim{}))
	claimed.Authority = memory.AuthorityOwnerStatement
	rejected["whole message claimed as owner words without its text"] = claimed
	for name, source := range rejected {
		if err := verifyRememberSourceEvidence(source, message); err == nil {
			t.Fatalf("%s was accepted: %+v", name, source)
		}
	}
}

// bindLiteral binds a Typed Literal Claim about the owner the way a remember
// proposal does.
func bindLiteral(message string, value memory.TypedLiteral, predicate string, polarity memory.ClaimPolarity) ownerSourceBinding {
	return bindOwnerSource(message, literalBindingClaim(message, value, polarity, predicate, strings.ReplaceAll(predicate, "_", " ")))
}

// Harness review final pass (M5, finding 1): the span is the sentence that
// states the Claim, not the first sentence that happens to contain its value.
func TestOwnerSpanBindingChoosesTheSentenceThatStatesTheClaim(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		literal       memory.TypedLiteral
		predicate     string
		span          string
	}{
		{"memory cue beats an earlier mention", "My therapist in Boston says the panic attacks are getting worse. Remember that I live in Boston.", text("Boston"), "home_city", "Remember that I live in Boston."},
		{"memory cue beats a later mention", "Remember that I live in Boston. My therapist in Boston says the panic attacks are getting worse.", text("Boston"), "home_city", "Remember that I live in Boston."},
		{"Predicate words beat an earlier mention", "I flew to Boston last week. My home city is Boston.", text("Boston"), "home_city", "My home city is Boston."},
		{"Predicate words beat a later mention", "My home city is Boston. I flew to Boston yesterday.", text("Boston"), "home_city", "My home city is Boston."},
		// Confirmation review: equal length, so recency decides; a shorter
		// sentence would win first (TestOwnerSpanBindingFollowsTheClauseSubject).
		{"later statement wins a tie", "I was in Boston on Monday. I'm in Boston again today.", text("Boston"), "home_city", "I'm in Boston again today."},
		{"statement beats a question", "Is Boston nice? I live in Boston.", text("Boston"), "home_city", "I live in Boston."},
		{"no sentence states it", "The Boston office called. Lunch was fine.", text("Boston"), "home_city", ""},
		{"topic mention without a cue", "Read this article about Boston.", text("Boston"), "home_city", ""},
		{"explicit memory cue", "Good question. Remember: Blue Bottle.", text("Blue Bottle"), "favorite_cafe", "Remember: Blue Bottle."},
		{"explicit correction cue", "Correction: it was Chicago.", text("Chicago"), "location_name", "Correction: it was Chicago."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBinding(t, tc.message, bindLiteral(tc.message, tc.literal, tc.predicate, memory.PolarityAffirmed), tc.span)
		})
	}
}

// Harness review final pass (M5, finding 2): a stray number word, a topic
// mention or a negated statement is not the owner stating the value.
func TestOwnerSpanBindingDoesNotLaunderAuthority(t *testing.T) {
	boolean := func(value string) memory.TypedLiteral { return literal(memory.LiteralBoolean, value) }
	integer := func(value string) memory.TypedLiteral { return literal(memory.LiteralInteger, value) }
	for _, tc := range []struct {
		name, message string
		literal       memory.TypedLiteral
		predicate     string
		polarity      memory.ClaimPolarity
		span          string
	}{
		{"number word without quantity context", "Read this one article about school schedules.", integer("1"), "kids", memory.PolarityAffirmed, ""},
		{"number word in an unrelated sentence", "No one told me. My floor is 4.", integer("1"), "floor", memory.PolarityAffirmed, ""},
		{"digit with its Predicate", "No one told me. My floor is 4.", integer("4"), "floor", memory.PolarityAffirmed, "My floor is 4."},
		{"number word with its Predicate", "One of my kids is sick today. I have two kids.", integer("2"), "kids", memory.PolarityAffirmed, "I have two kids."},
		{"boolean topic mention", "Read this article about peanut allergy treatments and summarize it.", boolean("true"), "has_peanut_allergy", memory.PolarityAffirmed, ""},
		{"boolean stated by the owner", "Note for later. I have a peanut allergy.", boolean("true"), "has_peanut_allergy", memory.PolarityAffirmed, "I have a peanut allergy."},
		{"boolean asked about", "Am I vegetarian?", boolean("true"), "is_vegetarian", memory.PolarityAffirmed, ""},
		{"negated boolean", "I'm not allergic to peanuts.", boolean("true"), "allergic", memory.PolarityAffirmed, ""},
		{"negated text value", "I'm not allergic to peanuts.", text("peanuts"), "allergy", memory.PolarityAffirmed, ""},
		{"negated home", "I don't live in Boston anymore.", text("Boston"), "home_city", memory.PolarityAffirmed, ""},
		{"no longer", "I no longer live in Boston.", text("Boston"), "home_city", memory.PolarityAffirmed, ""},
		{"denied Claim from a negated statement", "I don't live in Boston anymore.", text("Boston"), "home_city", memory.PolarityDenied, "I don't live in Boston anymore."},
		{"denied Claim from a positive statement", "I live in Boston.", text("Boston"), "home_city", memory.PolarityDenied, ""},
		{"false boolean from a negated statement", "I'm not vegetarian.", boolean("false"), "is_vegetarian", memory.PolarityAffirmed, "I'm not vegetarian."},
		{"negation in another clause", "I'm vegetarian, not vegan.", boolean("true"), "is_vegetarian", memory.PolarityAffirmed, "I'm vegetarian, not vegan."},
		{"negation in a clause after but", "I don't drive, but I live in Boston.", text("Boston"), "home_city", memory.PolarityAffirmed, "I don't drive, but I live in Boston."},
		{"negation in a clause after so", "I prefer a printed errand list so I do not have to unlock a screen while I am out.", text("a printed errand list"), "errand_list_medium", memory.PolarityAffirmed, "I prefer a printed errand list so I do not have to unlock a screen while I am out."},
		{"so as an adverb keeps the clause", "I'm not so sure I live in Boston.", text("Boston"), "home_city", memory.PolarityAffirmed, ""},
		{"negation in a clause after because", "I live in Boston because I don't like driving.", text("Boston"), "home_city", memory.PolarityAffirmed, "I live in Boston because I don't like driving."},
		{"don't forget is not a negation", "Don't forget that I live in Boston.", text("Boston"), "home_city", memory.PolarityAffirmed, "Don't forget that I live in Boston."},
		{"negated sentence skipped for the stated one", "I don't live in Boston anymore. I live in Boston again now.", text("Boston"), "home_city", memory.PolarityAffirmed, "I live in Boston again now."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBinding(t, tc.message, bindLiteral(tc.message, tc.literal, tc.predicate, tc.polarity), tc.span)
		})
	}
}

// Confirmation review (M5): the rules are deterministic heuristics, so each
// fails in the safe direction. Unsure authority is Evie-proposed (approval is
// still required; only the label changes), and among qualifying sentences the
// shortest wins before the latest, so the least extra text is quoted. These
// are the reviewer's messages.
func TestOwnerSpanBindingFollowsTheClauseSubject(t *testing.T) {
	boolean := func(value string) memory.TypedLiteral { return literal(memory.LiteralBoolean, value) }
	integer := func(value string) memory.TypedLiteral { return literal(memory.LiteralInteger, value) }
	affirmed, denied := memory.PolarityAffirmed, memory.PolarityDenied
	for _, tc := range []struct {
		name, message string
		literal       memory.TypedLiteral
		predicate     string
		polarity      memory.ClaimPolarity
		span          string
	}{
		// Defect 1: a later sentence about someone else that names the value is
		// not the owner stating it, however recent.
		{"therapist after the statement", "I live in Boston. My therapist in Boston says the panic attacks are getting worse.", text("Boston"), "home_city", affirmed, "I live in Boston."},
		{"oncologist after the statement", "I'm based in Boston. My oncologist in Boston wants another scan next week.", text("Boston"), "home_city", affirmed, "I'm based in Boston."},
		{"divorce lawyer after the statement", "We moved to Boston last year. My divorce lawyer in Boston says the hearing is in May.", text("Boston"), "home_city", affirmed, "We moved to Boston last year."},
		{"psychiatrist after a cue", "Remember I live in Boston. My psychiatrist in Boston upped my Zoloft dose.", text("Boston"), "home_city", affirmed, "Remember I live in Boston."},
		{"manager after the statement", "I work at Initech. My manager at Initech put me on a performance plan.", text("Initech"), "employer", affirmed, "I work at Initech."},
		{"retire request with a later mention", "Forget that I live in Boston. My therapist in Boston says the panic attacks are getting worse.", text("Boston"), "home_city", affirmed, "Forget that I live in Boston."},
		{"shortest span before the latest", "I am in Boston today. I was in Boston for the whole conference week.", text("Boston"), "home_city", affirmed, "I am in Boston today."},
		{"possessor of the Predicate is someone else", "My dad's favorite color is teal.", text("teal"), "favorite_color", affirmed, ""},
		{"named possessor", "Selma's favorite color is teal.", text("teal"), "favorite_color", affirmed, ""},
		// Defect 4: plainly stated owner answers keep owner authority.
		{"terse answer", "Boston.", text("Boston"), "home_city", affirmed, "Boston."},
		{"terse answer with it", "It's Boston.", text("Boston"), "home_city", affirmed, "It's Boston."},
		{"terse answer with please", "Teal, please.", text("teal"), "favorite_color", affirmed, "Teal, please."},
		{"terse digit", "3", integer("3"), "kids", affirmed, "3"},
		{"terse date", "June 13.", literal(memory.LiteralDate, "2026-06-13"), "sister_birthday", affirmed, "June 13."},
		{"terse correction", "No, it's Chicago.", text("Chicago"), "home_city", affirmed, "No, it's Chicago."},
		{"terse correction with the old value", "Chicago, not Boston.", text("Chicago"), "home_city", affirmed, "Chicago, not Boston."},
		{"terse correction with actually", "Actually Chicago.", text("Chicago"), "home_city", affirmed, "Actually Chicago."},
		{"negation after and I", "I live in Boston and I don't plan to move.", text("Boston"), "home_city", affirmed, "I live in Boston and I don't plan to move."},
		{"negation after and I with Predicate words", "My favorite color is teal and I don't like red.", text("teal"), "favorite_color", affirmed, "My favorite color is teal and I don't like red."},
		{"negation after and an auxiliary", "Remember that I'm allergic to peanuts and can't eat satay.", boolean("true"), "allergic_to_peanuts", affirmed, "Remember that I'm allergic to peanuts and can't eat satay."},
		{"change word after and an auxiliary", "I work at Initech and haven't left yet.", text("Initech"), "employer", affirmed, "I work at Initech and haven't left yet."},
		{"owner reports her own words", "I can't wait to tell you I live in Boston now.", text("Boston"), "home_city", affirmed, "I can't wait to tell you I live in Boston now."},
		{"not X after the value", "I live in Cambridge not Boston.", text("Cambridge"), "home_city", affirmed, "I live in Cambridge not Boston."},
		{"the negated value after it", "I live in Cambridge not Boston.", text("Boston"), "home_city", affirmed, ""},
		// Defect 5: third-party, reported, conditional, wished, asked and past
		// statements are not the owner stating a present value.
		{"third-party boolean", "My sister is vegetarian.", boolean("true"), "is_vegetarian", affirmed, ""},
		{"third-party value", "My sister lives in Boston.", text("Boston"), "home_city", affirmed, ""},
		{"third party reporting a third party", "My sister says she is vegetarian.", boolean("true"), "is_vegetarian", affirmed, ""},
		{"single-quoted reported speech", "My friend said 'I live in Boston'.", text("Boston"), "home_city", affirmed, ""},
		{"double-quoted reported speech", "My friend said \"I live in Boston\" and laughed.", text("Boston"), "home_city", affirmed, ""},
		{"reported without quotes", "The doctor says I'm allergic to peanuts.", boolean("true"), "allergic_to_peanuts", affirmed, ""},
		{"reported by an article", "The article says I'm allergic to peanuts.", boolean("true"), "allergic_to_peanuts", affirmed, ""},
		{"question then report", "Am I allergic to peanuts? The doctor says I'm allergic to peanuts.", boolean("true"), "allergic_to_peanuts", affirmed, ""},
		{"conditional with a comma", "If I lived in Boston, I'd take the T.", text("Boston"), "home_city", affirmed, ""},
		{"conditional without a comma", "If I lived in Boston I'd take the T.", text("Boston"), "home_city", affirmed, ""},
		{"wish", "I wish I lived in Boston.", text("Boston"), "home_city", affirmed, ""},
		{"question", "Should I move to Boston?", text("Boston"), "home_city", affirmed, ""},
		{"might", "I might move to Boston.", text("Boston"), "home_city", affirmed, ""},
		{"would never, denied", "I would never live in Boston.", text("Boston"), "home_city", denied, ""},
		{"left for good", "I left Boston for good.", text("Boston"), "home_city", affirmed, ""},
		{"used to", "I used to live in Boston.", text("Boston"), "home_city", affirmed, ""},
		{"asked and denied", "I have 2 kids? No.", integer("2"), "kids", affirmed, ""},
		{"hardly", "I'm hardly vegetarian.", boolean("true"), "is_vegetarian", affirmed, ""},
		{"double negation, affirmed", "It's not that I don't live in Boston.", text("Boston"), "home_city", affirmed, ""},
		{"double negation, denied", "It's not that I don't live in Boston.", text("Boston"), "home_city", denied, ""},
		{"double negation with true, denied", "It isn't true that I don't live in Boston.", text("Boston"), "home_city", denied, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBinding(t, tc.message, bindLiteral(tc.message, tc.literal, tc.predicate, tc.polarity), tc.span)
		})
	}
}

// Confirmation review (defect 4): an Entity Claim whose subject is the named
// Entity binds when that name is its clause's subject; a negation in a later
// clause does not reach it, and someone else reporting it is not the owner.
func TestOwnerSpanBindingForEntityClaimsFollowsTheClauseSubject(t *testing.T) {
	for _, tc := range []struct {
		message, span string
	}{
		{"Alex works at Initech and doesn't like it.", "Alex works at Initech and doesn't like it."},
		{"Alex works at Initech. He isn't happy there.", "Alex works at Initech."},
		{"My sister says Alex works at Initech.", ""},
		{"Bob said, Alex works at Initech.", ""},
	} {
		t.Run(tc.message, func(t *testing.T) {
			tokens := bindingTokens(tc.message)
			claim := bindingClaim{needs: []bindingOccurrences{phraseOccurrences(tokens, "Alex"), phraseOccurrences(tokens, "Initech")},
				predicate: newBindingPredicate("employer", "employer")}
			checkBinding(t, tc.message, bindOwnerSource(tc.message, claim), tc.span)
		})
	}
}
