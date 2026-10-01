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
		{"-ves plural", "Hi. Selma likes indigo cotton scarves.", text("indigo cotton scarf"), "keepsake", "Selma likes indigo cotton scarves."},
		{"-ies plural", "We love small cities. Ok.", text("small city"), "preference", "We love small cities."},
		{"-ves plural of -ve", "Bring gloves. Thanks.", text("glove"), "packing", "Bring gloves."},
		{"multi-sentence value", "Note this. Rule one! Rule two! Done", text("Rule one! Rule two!"), "rules", "Rule one! Rule two!"},
		{"possessive keeps the name", "Sarah's birthday party is Friday.", text("Sarah"), "friend", "Sarah's birthday party is Friday."},
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
			binding := bindOwnerSource(tc.message, literalOccurrences(tc.message, tc.literal, tc.predicate, strings.ReplaceAll(tc.predicate, "_", " ")))
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
			checkBinding(t, tc.message, bindOwnerSource(tc.message, needs), tc.span)
		})
	}
}

func TestOwnerSpanBindingNarrowsLongSentences(t *testing.T) {
	message := strings.Repeat("filler words without any stop ", 40) + "my locker code word is pelican " + strings.Repeat("and more trailing words here ", 40)
	binding := bindOwnerSource(message, literalOccurrences(message, text("pelican"), "locker_code_word", "locker code word"))
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
	owner := sourceFor(bindOwnerSource(message, literalOccurrences(message, text("teal"), "favorite_color", "favorite color")))
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
	quoting := sourceFor(bindOwnerSource(message, nil))
	quoting.Evidence = message
	rejected["Evie-proposed quoting the message"] = quoting
	claimed := sourceFor(bindOwnerSource(message, nil))
	claimed.Authority = memory.AuthorityOwnerStatement
	rejected["whole message claimed as owner words without its text"] = claimed
	for name, source := range rejected {
		if err := verifyRememberSourceEvidence(source, message); err == nil {
			t.Fatalf("%s was accepted: %+v", name, source)
		}
	}
}
