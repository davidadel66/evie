package memory

import "testing"

func TestHasExplicitMemoryCue(t *testing.T) {
	for message, want := range map[string]bool{
		"Remember that I prefer tea.":                 true,
		"please MEMORISE my bank's name":              true,
		"Save this: standups are at 9.":               true,
		"Note that the build needs Go 1.24.":          true,
		"Keep in mind I'm in Lisbon this week.":       true,
		"Don't forget my sister's birthday is May 3.": true,
		"Do not forget that I'm vegetarian.":          true,
		"Add this to your memory: I'm left-handed.":   true,
		"Store my shoe size: 44.":                     true,
		"I prefer tea.":                               false,
		"We decided to use SQLite for the cache.":     false,
		"Keep it brief.":                              false,
		"memorable trip, notebook ready":              false,
		"":                                            false,
	} {
		if got := HasExplicitMemoryCue(message); got != want {
			t.Errorf("HasExplicitMemoryCue(%q) = %v, want %v", message, got, want)
		}
	}
}
