package eviedb

import "testing"

func TestWordingRulesForNewerStatementsAndRestatements(t *testing.T) {
	var home claimWording
	home.addSubject("owner", true)
	home.addPredicate("home_city")
	home.addPredicate("home city")
	home.addValue("text", "Boston")
	var dentist claimWording
	dentist.addSubject("owner", true)
	dentist.addPredicate("dentist")
	dentist.addValue("text", "Dr. Okafor")
	var shoe claimWording
	shoe.addSubject("owner", true)
	shoe.addPredicate("shoe_size")
	shoe.addValue("text", "9") // a lone number cannot anchor a match
	var mom claimWording
	mom.addSubject("Mom", false)
	mom.addPredicate("lives_in")
	mom.addValue("text", "Boston")
	owned := func(token, label, value string) claimWording {
		var w claimWording
		w.addSubject("owner", true)
		w.addPredicate(token)
		w.addPredicate(label)
		w.addValue("text", value)
		return w
	}
	carrier := owned("phone_carrier", "phone carrier", "Verizon")
	barber := owned("barber", "barber", "Luis")
	coffee := owned("favorite_coffee_shop", "favorite coffee shop", "Blue Bottle")
	parking := owned("parking_spot", "parking spot", "level 2 bay 14")
	for _, tc := range []struct {
		name    string
		wording claimWording
		rule    string
		text    string
		want    bool
	}{
		{"value and change cue", home, "newer", "Big news: I moved to Chicago last month, Boston is behind me.", true},
		{"value without first person", home, "newer", "The Boston marathon moved to a new date this year.", false},
		{"value without change cue", home, "newer", "My Boston friends are visiting next week.", false},
		{"question", home, "newer", "Should I move back to Boston now?", false},
		{"cue in another sentence", home, "newer", "I love Boston. Now plan my week.", false},
		{"predicate words in another order", shoe, "newer", "My shoes are a size 10 now after the running season.", true},
		{"one of two predicate words", shoe, "newer", "I need new running shoes before the 10k.", false},
		{"abbreviation stays in one sentence", dentist, "restatement", "My dentist is Dr. Okafor again.", true},
		{"restatement with a predicate word", home, "restatement", "Boston is still my home, honestly.", true},
		{"value without predicate words", home, "restatement", "Boston has great seafood near the harbor.", false},
		{"non-owner subject name", mom, "restatement", "Reminder that Mom is in Boston these days.", true},
		{"non-owner subject absent", mom, "newer", "I moved to Boston now.", false},
		// Final verification pass (M3): the owner must be the subject of the
		// clause carrying the cue, and the cue must govern the value or the
		// Predicate's words, with only light words between them.
		{"third-party subject", home, "newer", "My sister moved to Boston now.", false},
		{"cue governs another object", home, "newer", "I left my umbrella in Boston.", false},
		{"cue governs a pronoun object", home, "newer", "I left it in Boston.", false},
		{"cue governs the value", home, "newer", "I left Boston for good last spring.", true},
		{"value as subject of the cue clause", home, "newer", "I'm in Chicago, and Boston is no longer home.", true},
		{"cue clause names a third party", home, "newer", "I love Boston, but my sister moved there now.", false},
		{"value with an unrelated cue", home, "newer", "My Boston friends are visiting now.", false},
		{"dropped governs the value", carrier, "newer", "I finally dropped Verizon last week and switched to T-Mobile.", true},
		{"novelty beside the Predicate word", barber, "newer", "My new barber is Marco.", true},
		{"predicate words with a number between", shoe, "newer", "My shoes are a size 10 now after the running season.", true},
		// The same subject requirement applies to restatements of an owner
		// Claim: a third party's preference or news about the value is not the
		// owner repeating the retired fact.
		{"third-party restatement", coffee, "restatement", "My friend Sam says his favorite coffee is Blue Bottle.", false},
		{"news about the value", coffee, "restatement", "The Blue Bottle coffee shop on Main Street closed today.", false},
		{"owner restatement", coffee, "restatement", "Honestly, Blue Bottle is still my favorite coffee shop.", true},
		{"owner restatement across clauses", parking, "restatement", "I finally got a parking spot at the office: level 2, bay 14.", true},
	} {
		rule := tc.wording.newerStatement
		if tc.rule == "restatement" {
			rule = tc.wording.restatement
		}
		if _, got := firstSentence(tc.text, 0, len(tc.text), rule); got != tc.want {
			t.Errorf("%s: %s(%q) = %v, want %v", tc.name, tc.rule, tc.text, got, tc.want)
		}
	}
	sentences := wordingSentences("Dr. Okafor said 2.4 bar. Is that right? Yes")
	if len(sentences) != 3 || !sentences[1].question || sentences[2].question {
		t.Fatalf("sentence split = %+v", sentences)
	}
}
