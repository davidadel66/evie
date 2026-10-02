package memoryeval

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
)

// ScaleCorpusVersion identifies the generated history, labels, and probes.
// Change it whenever generated content or gold labels change so a baseline is
// never compared across different corpora.
const ScaleCorpusVersion = "memory-scale-replay-v4"

const (
	ScaleTierDefault = "default"
	ScaleTierLarge   = "large"

	// Automatic Recall on a new user message, and the two model-directed read
	// tools measured one at a time. A turn holds at most eight evidence items,
	// so calling both tools at once would let one tool's results evict the
	// other's and hide which generator found what.
	ScalePathAutomatic          = "automatic"
	ScalePathMemorySearch       = "memory_search"
	ScalePathConversationSearch = "memory_search_conversations"

	ScaleAreaGlobal = "global"

	ScaleRoleOwner     = "owner"
	ScaleRoleAssistant = "assistant"

	// ScaleSessionKeyPrefix labels evidence from the probe's own session. Those
	// messages are already in the provider request, so re-injecting them adds
	// no information; the scorer counts them as unwanted same-session items.
	ScaleSessionKeyPrefix = "session:"
)

// Probe families. Each family has one measurement purpose; the report keeps
// them separate rather than averaging unlike questions into one score.
const (
	ScaleFamilyRelevant      = "relevant"        // ordinary request with a known needle
	ScaleFamilyOneWordAnswer = "one_word_answer" // ordinary question whose answer shares one of its words
	ScaleFamilyParaphrase    = "paraphrase"      // needle shares meaning, not words
	ScaleFamilyFollowUp      = "follow_up"       // short follow-up that needs earlier context
	ScaleFamilyLowContent    = "low_content"     // "thanks!" and similar: no new information need
	ScaleFamilyPrivacy       = "privacy"         // unrelated request sharing a word with private history
	ScaleFamilyUnrelated     = "unrelated"       // no personal memory needed
	ScaleFamilyStale         = "stale"           // corrected, retired, contradicted, or drifted facts
	ScaleFamilyDenseCoverage = "dense_coverage"  // large tier: dense-only targets across the vector table
)

// Stale-fact scenarios mirror harness-review rows M2, M3 and M4. Controls use
// the same mechanics with wording the current implementation already handles,
// so a failing control means the instrument, not the system, is wrong.
const (
	ScaleStaleCorrection       = "correction_old_source"
	ScaleStaleRetiredRestated  = "retired_restatement"
	ScaleStaleRetiredRepeat    = "retired_unlinked_repeat"
	ScaleStaleRetiredSource    = "retired_source_control"
	ScaleStaleNewerWording     = "contradiction_different_wording"
	ScaleStaleNewerSavedValue  = "contradiction_saved_value"
	ScaleStaleNewerPredicate   = "contradiction_predicate_words"
	ScaleStaleNewerSameWording = "contradiction_same_wording_control"
	ScaleStaleLabelDrift       = "predicate_label_drift"
	ScaleStaleCardinalityDrift = "predicate_cardinality_drift"
	ScaleStaleSamePredicate    = "same_predicate_conflict_control"
	// Precision checks (corpus v2): a later owner message that shares a saved
	// value or Predicate word but does not update the Claim must not be linked
	// to it, and a mention of a retired value that does not restate the Claim
	// must not be flagged as its restatement.
	ScaleStaleOverLink = "unrelated_statement_not_linked"
	ScaleStaleOverFlag = "unrelated_mention_not_flagged"
)

type ScaleCorpusOptions struct {
	Tier            string `json:"tier"`
	Seed            uint64 `json:"seed"`
	GlobalSessions  int    `json:"global_sessions"`
	ProjectSessions int    `json:"project_sessions"`
	MinExchanges    int    `json:"min_exchanges"`
	MaxExchanges    int    `json:"max_exchanges"`
	DenseTargets    int    `json:"dense_targets"`
}

// DefaultScaleCorpusOptions is the always-run tier: realistic history size,
// lexical retrieval only (the production default without an embedding
// endpoint), and fast enough for the repository verification script.
func DefaultScaleCorpusOptions() ScaleCorpusOptions {
	return ScaleCorpusOptions{Tier: ScaleTierDefault, Seed: 20261001, GlobalSessions: 96, ProjectSessions: 18, MinExchanges: 3, MaxExchanges: 9}
}

// LargeScaleCorpusOptions grows Global history past the 4,096-vector dense
// scan bound so dense coverage can be measured with a deterministic embedder.
func LargeScaleCorpusOptions() ScaleCorpusOptions {
	return ScaleCorpusOptions{Tier: ScaleTierLarge, Seed: 20261001, GlobalSessions: 400, ProjectSessions: 24, MinExchanges: 4, MaxExchanges: 9, DenseTargets: 24}
}

type ScaleArea struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type ScaleMessage struct {
	Key     string `json:"key"`
	Role    string `json:"role"`
	Topic   string `json:"topic"`
	Area    string `json:"area"`
	Private bool   `json:"private,omitempty"`
	Text    string `json:"text"`
}

type ScaleClaim struct {
	Key         string `json:"key"`
	Topic       string `json:"topic"`
	Predicate   string `json:"predicate"`
	Label       string `json:"label"`
	Cardinality string `json:"cardinality"`
	Value       string `json:"value"`
}

type ScaleStepKind string

const (
	ScaleStepExchange ScaleStepKind = "exchange"
	ScaleStepRemember ScaleStepKind = "remember"
	ScaleStepCorrect  ScaleStepKind = "correct"
	ScaleStepRetire   ScaleStepKind = "retire"
)

// ScaleStep is one durable owner action. Remember and correct steps record the
// owner command as the source event and then apply the approved Kernel
// operation; retire steps run the memory_retire tool through an approved turn.
type ScaleStep struct {
	Kind           ScaleStepKind `json:"kind"`
	Owner          ScaleMessage  `json:"owner"`
	Assistant      *ScaleMessage `json:"assistant,omitempty"`
	Claim          string        `json:"claim,omitempty"`
	Target         string        `json:"target,omitempty"`
	Mode           string        `json:"mode,omitempty"`
	IdempotencyKey string        `json:"idempotency_key,omitempty"`
}

type ScaleSession struct {
	Key   string      `json:"key"`
	Area  string      `json:"area"`
	Steps []ScaleStep `json:"steps"`
}

// ScaleStaleCheck is evaluated against one probe's delivered evidence.
// StaleKeys must not be delivered as unflagged current evidence. LinkKeys is a
// pair that must be connected by a conflict warning or relation. ClearKeys is
// a (Claim, message) pair that must not be connected: delivering the message
// linked or flagged against that Claim is over-linking.
type ScaleStaleCheck struct {
	Scenario  string   `json:"scenario"`
	Issue     string   `json:"issue"`
	Control   bool     `json:"control,omitempty"`
	StaleKeys []string `json:"stale_keys,omitempty"`
	LinkKeys  []string `json:"link_keys,omitempty"`
	ClearKeys []string `json:"clear_keys,omitempty"`
}

type ScaleProbe struct {
	ID       string            `json:"id"`
	Family   string            `json:"family"`
	Path     string            `json:"path"`
	Area     string            `json:"area"`
	Prelude  []string          `json:"prelude,omitempty"`
	Message  string            `json:"message"`
	Query    string            `json:"query,omitempty"`
	Topics   []string          `json:"topics,omitempty"`
	Required []string          `json:"required,omitempty"`
	Checks   []ScaleStaleCheck `json:"checks,omitempty"`
}

type ScaleCorpus struct {
	Version  string              `json:"version"`
	Options  ScaleCorpusOptions  `json:"options"`
	Areas    []ScaleArea         `json:"areas"`
	Sessions []ScaleSession      `json:"sessions"`
	Claims   []ScaleClaim        `json:"claims"`
	Probes   []ScaleProbe        `json:"probes"`
	Concepts map[string][]string `json:"concepts"`
}

// ConceptOf maps a lower-case word to its concept group for the test embedder.
func (c ScaleCorpus) ConceptOf() map[string]string {
	concepts := map[string]string{}
	for concept, words := range c.Concepts {
		for _, word := range words {
			concepts[word] = concept
		}
	}
	return concepts
}

// Digest identifies the exact generated corpus, including every label.
func (c ScaleCorpus) Digest() string {
	encoded, _ := json.Marshal(c)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
}

// Messages returns every generated message with its gold labels.
func (c ScaleCorpus) Messages() map[string]ScaleMessage {
	messages := make(map[string]ScaleMessage)
	for _, session := range c.Sessions {
		for _, step := range session.Steps {
			messages[step.Owner.Key] = step.Owner
			if step.Assistant != nil {
				messages[step.Assistant.Key] = *step.Assistant
			}
		}
	}
	return messages
}

func (c ScaleCorpus) Claim(key string) (ScaleClaim, bool) {
	for _, claim := range c.Claims {
		if claim.Key == key {
			return claim, true
		}
	}
	return ScaleClaim{}, false
}

// Validate rejects a corpus whose probes reference absent labels, so a typo
// cannot silently become an unreachable "required" item.
func (c ScaleCorpus) Validate() error {
	areas := make(map[string]bool)
	for _, area := range c.Areas {
		areas[area.Key] = true
	}
	messages := c.Messages()
	known := func(key string) bool {
		if _, ok := messages[key]; ok {
			return true
		}
		_, ok := c.Claim(key)
		return ok
	}
	seen := make(map[string]bool)
	for _, session := range c.Sessions {
		if !areas[session.Area] {
			return fmt.Errorf("session %s has unknown area %q", session.Key, session.Area)
		}
		for _, step := range session.Steps {
			for _, key := range []string{step.Owner.Key, assistantKey(step)} {
				if key == "" {
					continue
				}
				if seen[key] {
					return fmt.Errorf("duplicate message key %s", key)
				}
				seen[key] = true
			}
			switch step.Kind {
			case ScaleStepRemember, ScaleStepCorrect, ScaleStepRetire:
				if _, ok := c.Claim(step.Claim); !ok && step.Kind != ScaleStepRetire {
					return fmt.Errorf("step %s names unknown claim %s", step.Owner.Key, step.Claim)
				}
				if !strings.HasPrefix(step.IdempotencyKey, "idem:v1:") {
					return fmt.Errorf("step %s lacks an idempotency key", step.Owner.Key)
				}
			}
			if (step.Kind == ScaleStepCorrect || step.Kind == ScaleStepRetire) && !known(step.Target) {
				return fmt.Errorf("step %s targets unknown claim %s", step.Owner.Key, step.Target)
			}
			if step.Kind == ScaleStepCorrect && step.Mode != "changed" && step.Mode != "error" {
				return fmt.Errorf("correction %s has mode %q", step.Owner.Key, step.Mode)
			}
		}
	}
	words := make(map[string]string)
	for concept, group := range c.Concepts {
		for _, word := range group {
			if prior, ok := words[word]; ok && prior != concept {
				return fmt.Errorf("concept word %q belongs to both %s and %s", word, prior, concept)
			}
			words[word] = concept
		}
	}
	ids := make(map[string]bool)
	for _, probe := range c.Probes {
		if ids[probe.ID] {
			return fmt.Errorf("duplicate probe %s", probe.ID)
		}
		ids[probe.ID] = true
		if !areas[probe.Area] {
			return fmt.Errorf("probe %s has unknown area %q", probe.ID, probe.Area)
		}
		switch probe.Path {
		case ScalePathAutomatic:
			if probe.Query != "" {
				return fmt.Errorf("automatic probe %s must not script a tool query", probe.ID)
			}
		case ScalePathMemorySearch, ScalePathConversationSearch:
			if probe.Query == "" || len(probe.Prelude) > 0 {
				return fmt.Errorf("tool probe %s needs one query and no prelude", probe.ID)
			}
		default:
			return fmt.Errorf("probe %s has unknown path %q", probe.ID, probe.Path)
		}
		for _, key := range probe.Required {
			if !known(key) {
				return fmt.Errorf("probe %s requires unknown key %s", probe.ID, key)
			}
		}
		for _, check := range probe.Checks {
			for _, key := range append(append(append([]string(nil), check.StaleKeys...), check.LinkKeys...), check.ClearKeys...) {
				if !known(key) {
					return fmt.Errorf("probe %s check %s names unknown key %s", probe.ID, check.Scenario, key)
				}
			}
			kinds := 0
			for _, keys := range [][]string{check.StaleKeys, check.LinkKeys, check.ClearKeys} {
				if len(keys) > 0 {
					kinds++
				}
			}
			if kinds != 1 || len(check.LinkKeys) != 0 && len(check.LinkKeys) != 2 || len(check.ClearKeys) != 0 && len(check.ClearKeys) != 2 {
				return fmt.Errorf("probe %s check %s needs stale keys, two link keys or two clear keys", probe.ID, check.Scenario)
			}
			if len(check.ClearKeys) == 2 {
				if _, ok := c.Claim(check.ClearKeys[0]); !ok {
					return fmt.Errorf("probe %s check %s clear keys must start with a Claim", probe.ID, check.Scenario)
				}
			}
		}
	}
	return nil
}

func assistantKey(step ScaleStep) string {
	if step.Assistant == nil {
		return ""
	}
	return step.Assistant.Key
}

// scaleTopic is a synthetic conversational area. Text is written for this
// evaluation only; it is not derived from any real Evie history.
type scaleTopic struct {
	key     string
	area    string
	private bool
	weight  int
	owner   []string
	reply   []string
	slots   map[string][]string
}

var scaleProjectAreas = []ScaleArea{
	{Key: "project:payments", Name: "payments-service"},
	{Key: "project:gardenapp", Name: "garden-planner-app"},
	{Key: "project:ledger", Name: "household-ledger"},
}

var scaleDays = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
var scaleMonths = []string{"March", "April", "May", "June", "September", "October", "November"}

var scaleTopics = []scaleTopic{
	{key: "cooking", area: ScaleAreaGlobal, weight: 9,
		owner: []string{
			"Can you scale the {dish} recipe for {count} people?",
			"I'm out of {ingredient}; what can I use instead in the {dish}?",
			"Plan a grocery list for {dish} on {day}.",
			"The {dish} came out bland last time, how do I fix that?",
			"How long does {dish} keep in the fridge?",
			"Add {ingredient} to the shopping list for this week.",
			"What drink goes well with {dish}?",
			"I want to batch cook {dish} on {day} night, give me a timeline.",
			"Is {dish} a good dinner for guests on {day}?",
		},
		reply: []string{
			"For the {dish}, build the base early and taste for salt before serving.",
			"You can swap {ingredient} with something similar; adjust the amount gradually.",
			"Prep on {day}, cook in one batch, and portion it into containers.",
			"That keeps three to four days in a sealed container in the fridge.",
		},
		slots: map[string][]string{
			"dish":       {"lentil soup", "mushroom risotto", "chickpea curry", "vegetable lasagna", "black bean tacos", "tofu stir-fry", "shakshuka", "pumpkin gnocchi"},
			"ingredient": {"smoked paprika", "fresh basil", "coconut milk", "miso paste", "feta", "tahini", "leeks", "sourdough"},
			"count":      {"two", "four", "six", "eight"},
			"day":        scaleDays,
		}},
	{key: "gardening", area: ScaleAreaGlobal, weight: 9,
		owner: []string{
			"When should I {task} {plant}?",
			"There are {pest} all over {plant} again, what is the gentlest fix?",
			"Is it too late in {season} to {task} {plant}?",
			"Which branch of {plant} should I cut back first?",
			"The leaves on {plant} are yellowing at the edges.",
			"Make a watering schedule for {plant} while I'm away next week.",
			"Found a bug on {plant} that looks like a tiny beetle; should I worry?",
			"How much sun should {plant} get in {season}?",
		},
		reply: []string{
			"For {plant}, {task} early in {season} and avoid doing it in a heat wave.",
			"{pest} usually respond to a soap spray; check the undersides of the leaves.",
			"Yellow edges often mean uneven watering; check drainage before feeding.",
			"Cut the weakest branch first, just above an outward-facing bud.",
		},
		slots: map[string][]string{
			"plant":  {"the tomato plants", "the basil", "the rosemary hedge", "the fig tree", "the dahlias", "the lavender", "the courgettes", "the apple tree"},
			"pest":   {"aphids", "slugs", "whitefly", "powdery mildew", "vine weevils"},
			"task":   {"prune", "repot", "feed", "water", "mulch", "stake"},
			"season": {"spring", "summer", "autumn", "winter"},
		}},
	{key: "running", area: ScaleAreaGlobal, weight: 7,
		owner: []string{
			"Plan the {workout} for {day} morning.",
			"I have {issue} after the {workout}, should I rest?",
			"What effort should my {workout} be if I'm training for a {distance}?",
			"Log today's {workout}: felt {feeling} for the first half.",
			"Should I race the {distance} if I still have {issue}?",
			"Build me a four-week plan that peaks with a {distance}.",
		},
		reply: []string{
			"Keep the {workout} controlled and stop early if you notice {issue} again.",
			"A sensible week mixes one {workout}, one long run and two easy days.",
			"Logged. Next time, note your average heart rate for comparison.",
		},
		slots: map[string][]string{
			"workout":  {"interval run", "tempo run", "long run", "hill repeats", "recovery jog", "track session"},
			"distance": {"5k", "10k", "15k", "trail race"},
			"issue":    {"tight calves", "a sore knee", "shin splints", "blisters"},
			"feeling":  {"strong", "sluggish", "relaxed", "heavy-legged"},
			"day":      scaleDays,
		}},
	{key: "travel", area: ScaleAreaGlobal, weight: 7,
		owner: []string{
			"Compare trains and flights to {city} in {month}.",
			"Remind me to pack my {item} for the {city} trip.",
			"Which neighbourhoods are good to stay in for {city}?",
			"Draft a three-day itinerary for {city} in {month}.",
			"Is {month} a rainy month in {city}?",
			"How do I get from the airport to the hotel in {city}?",
		},
		reply: []string{
			"In {month}, {city} is usually mild; bring a {item} just in case.",
			"Day one: old town walk; day two: museums; day three: food market and a viewpoint.",
			"Trains are slower but skip the airport transfers; compare door-to-door times.",
		},
		slots: map[string][]string{
			"city":  {"Porto", "Kyoto", "Montreal", "Edinburgh", "Oaxaca", "Vienna", "Seville"},
			"item":  {"passport", "rain jacket", "adapter plug", "walking shoes", "travel pillow"},
			"month": scaleMonths,
		}},
	{key: "home", area: ScaleAreaGlobal, weight: 6,
		owner: []string{
			"The {fixture} {problem}, can I fix it myself?",
			"Do I need a {tool} to replace the {fixture}?",
			"Add checking the {fixture} to the weekend list.",
			"How much should a plumber charge to look at the {fixture}?",
			"The {fixture} test button does nothing.",
		},
		reply: []string{
			"Turn off the supply first, then inspect the {fixture} for worn seals.",
			"A {tool} helps, but most of this job needs only basic hand tools.",
			"If the {fixture} {problem} after a reset, call a professional.",
		},
		slots: map[string][]string{
			"fixture": {"kitchen tap", "bathroom fan", "garage door", "smoke alarm", "boiler", "window latch", "dishwasher"},
			"tool":    {"adjustable wrench", "stud finder", "multimeter", "caulk gun"},
			"problem": {"drips constantly", "makes a grinding noise", "won't close properly", "keeps beeping", "shows an error code"},
		}},
	{key: "reading", area: ScaleAreaGlobal, weight: 5,
		owner: []string{
			"Add {book} to my reading list.",
			"I finished {book}; recommend something similar.",
			"What's a good {genre} book for a long flight?",
			"Summarize the themes of {book} without spoilers.",
			"Book club picked {book}; draft three discussion questions.",
		},
		reply: []string{
			"If you liked {book}, try another {genre} title with a similar pace.",
			"Three questions: what does the setting represent, who changes most, and what is left unresolved?",
		},
		slots: map[string][]string{
			"book":  {"The Overstory", "Piranesi", "Middlemarch", "The Dispossessed", "Braiding Sweetgrass", "A Gentleman in Moscow"},
			"genre": {"science fiction", "nature writing", "historical fiction", "essays"},
		}},
	{key: "money", area: ScaleAreaGlobal, weight: 5,
		owner: []string{
			"How much did I spend on {category} in {month}?",
			"Set a {amount} euro monthly budget for {category}.",
			"Check whether the {category} budget is on track this week.",
			"Review my {category} spending and suggest one cut.",
		},
		reply: []string{
			"Your {category} spending looks close to the {amount} euro target.",
			"One easy cut: review recurring {category} charges at the start of the month.",
		},
		slots: map[string][]string{
			"category": {"groceries", "eating out", "subscriptions", "transport", "utilities", "gifts"},
			"amount":   {"40", "75", "120", "200", "350"},
			"month":    scaleMonths,
		}},
	{key: "car", area: ScaleAreaGlobal, weight: 5,
		owner: []string{
			"The car makes {noise}; is it safe to drive?",
			"When should I replace the {part}?",
			"Book the car service before the {km} km check.",
			"How do I check the {part} myself?",
			"Is the battery test at the garage worth paying for?",
		},
		reply: []string{
			"That is worth checking soon; avoid long trips until a mechanic looks at it.",
			"Most {part} last a few years; check wear at each service.",
		},
		slots: map[string][]string{
			"part":  {"brake pads", "wiper blades", "cabin filter", "winter tyres", "battery"},
			"noise": {"a squeal when braking", "a rattle at low speed", "a clicking sound when turning"},
			"km":    {"45,000", "60,000", "75,000"},
		}},
	{key: "family", area: ScaleAreaGlobal, weight: 6,
		owner: []string{
			"What should I get {person} for their {event}?",
			"Remind me to call {person} on Sunday.",
			"Plan a small dinner for {person}'s {event}.",
			"Would {idea} be a good present for {person}?",
		},
		reply: []string{
			"{idea} is a thoughtful choice for a {event}.",
			"I'll set a reminder for Sunday afternoon to call {person}.",
		},
		slots: map[string][]string{
			"person": {"my sister", "my dad", "my nephew", "my niece", "my aunt"},
			"event":  {"birthday", "graduation", "anniversary", "housewarming"},
			"idea":   {"a cooking class", "a framed photo", "a board game", "concert tickets", "a plant"},
		}},
	{key: "pets", area: ScaleAreaGlobal, weight: 5,
		owner: []string{
			"Biscuit {behaviour}; any training tips?",
			"Order more {thing} for Biscuit.",
			"How often should Biscuit get {thing}?",
			"Biscuit seems tired after the long walk.",
		},
		reply: []string{
			"Short daily sessions with treats help when a dog {behaviour}.",
			"Monthly is typical for {thing}, but follow the label.",
		},
		slots: map[string][]string{
			"thing":     {"the harness", "chew toys", "the food bowl", "flea treatment", "the crate"},
			"behaviour": {"barks at the door", "pulls on the lead", "chews the sofa", "won't settle at night"},
		}},
	{key: "health", area: ScaleAreaGlobal, private: true, weight: 3,
		owner: []string{
			"I've had {symptom} for a few days; should I book {appointment}?",
			"Remind me about the appointment with {appointment} on Thursday.",
			"Slept badly again; {symptom} is back.",
		},
		reply: []string{
			"If {symptom} persists for more than a week, booking {appointment} makes sense.",
			"I'll remind you on Wednesday evening.",
		},
		slots: map[string][]string{
			"symptom":     {"a headache", "trouble sleeping", "a stiff back", "low energy"},
			"appointment": {"the GP", "the physio", "the optician"},
		}},
	{key: "relationship", area: ScaleAreaGlobal, private: true, weight: 2,
		owner: []string{
			"Sam and I need to talk about the holidays and whose family we visit.",
			"I'm feeling a bit lonely this week.",
			"Sam forgot our plans again and I didn't say anything.",
		},
		reply: []string{
			"That sounds hard. Would it help to plan what you want to say?",
			"It might help to pick a calm moment to talk it through.",
		},
	},
	{key: "payments", area: "project:payments", weight: 1,
		owner: []string{
			"The {test} test failed in {env}; can you look at the logs?",
			"Write a unit test for the {component}.",
			"Why is the {component} slow in {env}?",
			"Summarize the test results from the last CI run.",
			"Open a branch for the {component} fix.",
			"There's a bug in the {component} when the amount is zero.",
			"Review the migration for the {component} before we deploy.",
			"The deploy to {env} failed on the health check.",
		},
		reply: []string{
			"The {test} failure looks timing related; use a fake clock in the test.",
			"I'd start by profiling the {component} and checking the database indexes.",
			"CI results: most suites passed and one test needs a rerun.",
		},
		slots: map[string][]string{
			"test":      {"TestCaptureRetry", "TestWebhookSignature", "TestLedgerBalance", "TestPayoutSchedule", "TestCurrencyRounding"},
			"component": {"webhook handler", "payout worker", "ledger service", "refund API", "rate limiter"},
			"env":       {"staging", "production", "CI"},
		}},
	{key: "gardenapp", area: "project:gardenapp", weight: 1,
		owner: []string{
			"Add a card for {plant} to the planner UI.",
			"The watering schedule screen crashes when there are no plants.",
			"Write a test for the frost date calculator.",
			"Rename the prune action to trim in the {plant} view.",
			"The {season} sowing calendar shows the wrong week.",
		},
		reply: []string{
			"Updated the component; the {plant} card now shows watering and pruning dates.",
			"Fixed the empty state; the screen now renders a hint instead of crashing.",
		},
		slots: map[string][]string{
			"plant":  {"tomato plants", "basil", "lavender", "courgettes"},
			"season": {"spring", "summer", "autumn"},
		}},
	{key: "ledger", area: "project:ledger", weight: 1,
		owner: []string{
			"Import the {month} bank statement CSV into the ledger.",
			"The {category} column totals don't match the bank.",
			"Add a chart for {category} by month.",
			"Fix the relationship between the accounts and transactions tables.",
		},
		reply: []string{
			"Imported; two rows need manual review because of duplicate references.",
			"The {category} totals differ because of a pending card payment.",
		},
		slots: map[string][]string{
			"category": {"groceries", "transport", "utilities", "eating out"},
			"month":    scaleMonths,
		}},
}

// Ordinary asides vary otherwise repeated requests and add the cross-topic
// common words that real conversations share.
var scaleAsides = []string{" No rush.", " Keep it short.", " Thanks in advance.", " This is for the weekend.", " Quick question.", " Same as last time if possible.", " I'm on my phone.", " Before Friday ideally."}

var scaleLowContent = []string{"thanks!", "ok", "great, thanks!", "sounds good", "perfect", "got it, thanks", "that worked, thanks", "thanks, that helps", "ok thanks"}
var scaleLowContentReplies = []string{"You're welcome!", "Glad that helped.", "Anytime.", "Happy to help."}

// scaleNeedle is a specific owner statement a probe must be able to recover.
type scaleNeedle struct {
	key, topic, area, owner, reply string
	private                        bool
	at                             float64
}

var scaleNeedles = []scaleNeedle{
	{key: "n.greenhouse", topic: "gardening", owner: "The greenhouse trial uses saffron crocuses and it is still experimental.", reply: "Noted; I'll treat the saffron crocus trial as experimental.", at: .14},
	{key: "n.basil", topic: "gardening", owner: "The basil on the balcony bolts every July unless I pinch off the flowers.", reply: "Pinching the flower buds early keeps the basil leafy.", at: .21},
	{key: "n.mother", topic: "family", owner: "My mother loves orchids but hates scented candles, so skip candles for her birthday.", reply: "Orchids it is; no scented candles.", at: .27},
	{key: "n.tyres", topic: "car", owner: "The car's tyre pressure should be 2.4 bar front and 2.2 bar rear.", reply: "Saved for the next time you check the tyres.", at: .33},
	{key: "n.pace", topic: "running", owner: "My half-marathon goal pace is 5:10 per kilometre.", reply: "That is a solid target; we can build tempo sessions around it.", at: .39},
	{key: "n.lisbon", topic: "travel", owner: "I booked the Lisbon hotel near Alfama for October 12 to 16.", reply: "Great, the Alfama hotel dates are October 12 to 16.", at: .46},
	{key: "n.cashew", topic: "cooking", owner: "I have a cashew allergy, so keep cashews out of every recipe.", reply: "Understood; I'll leave cashews out of recipes.", at: .52},
	{key: "n.vet", topic: "pets", owner: "Biscuit's annual vet appointment is every March for his booster.", reply: "I'll remember the March booster appointment.", at: .58},
	{key: "n.ferritin", topic: "health", private: true, owner: "My blood test results came back and my ferritin is low; the doctor wants a follow-up in six weeks.", reply: "I'm sorry to hear that; a follow-up in six weeks sounds sensible.", at: .63},
	{key: "n.pressure", topic: "health", private: true, owner: "The blood pressure reading this morning was 128 over 84.", reply: "Logged the morning reading.", at: .69},
	{key: "n.sam", topic: "relationship", private: true, owner: "Sam and I argued again last night; honestly I'm not sure our relationship is going to last.", reply: "That sounds painful. Do you want to talk through what happened?", at: .74},
	{key: "n.flaky", topic: "payments", area: "project:payments", owner: "The flaky test in payments is TestRefundIdempotency; it fails when the clock skews.", reply: "I'll pin the clock in TestRefundIdempotency.", at: .40},
	{key: "n.revolut", topic: "ledger", area: "project:ledger", owner: "The ledger import treats the Revolut CSV dates as month-first.", reply: "I'll parse Revolut dates as month-first.", at: .55},
	// Same words as the Global greenhouse needle, but in another Context Scope:
	// it must never reach a Global probe regardless of lexical strength.
	{key: "n.greenhouse_app", topic: "gardenapp", area: "project:gardenapp", owner: "The greenhouse trial screen in the app needs a saffron crocus icon.", reply: "Added a saffron crocus icon to the greenhouse trial screen.", at: .50},
	// Corpus v3 (harness review final pass): answers that share only one
	// content word with an ordinary question about them.
	{key: "n.passport", topic: "travel", owner: "My passport expires in March 2029.", reply: "Okay, noted.", at: .30},
	{key: "n.parked", topic: "car", owner: "I parked on level 3, row F of the Elm Street garage.", reply: "Okay, noted.", at: .35},
	{key: "n.sister", topic: "family", owner: "My sister Lena was born on June 13, 1994.", reply: "Okay, noted.", at: .44},
	{key: "n.cat_vet", topic: "pets", owner: "Our vet is Dr. Rivera at Oakwood Animal Clinic.", reply: "Okay, noted.", at: .48},
	{key: "n.wifi", topic: "home", owner: "The home wifi is called Evergreen5G and the key is taped under the router.", reply: "Okay, noted.", at: .53},
}

// scaleScenario is a fixed Global session at a relative point in history.
type scaleScenario struct {
	at    float64
	steps []ScaleStep
}

var scaleClaims = []ScaleClaim{
	{Key: "c.dinner", Topic: "cooking", Predicate: "dinner_preference", Label: "dinner preference", Cardinality: "one", Value: "vegetarian dinners"},
	{Key: "c.coffee", Topic: "coffee", Predicate: "favorite_coffee_shop", Label: "favorite coffee shop", Cardinality: "one", Value: "Blue Bottle"},
	{Key: "c.employer", Topic: "employer", Predicate: "employer", Label: "employer", Cardinality: "one", Value: "Initech"},
	{Key: "c.employer.new", Topic: "employer", Predicate: "employer", Label: "employer", Cardinality: "one", Value: "Globex"},
	{Key: "c.home", Topic: "home_city", Predicate: "home_city", Label: "home city", Cardinality: "one", Value: "Boston"},
	{Key: "c.gym", Topic: "gym", Predicate: "gym", Label: "gym", Cardinality: "one", Value: "Equinox"},
	{Key: "c.dentist", Topic: "dentist", Predicate: "dentist", Label: "dentist", Cardinality: "one", Value: "Dr. Patel"},
	{Key: "c.dentist.drift", Topic: "dentist", Predicate: "dentist", Label: "dental provider", Cardinality: "one", Value: "Dr. Okafor"},
	{Key: "c.bank", Topic: "bank", Predicate: "primary_bank", Label: "primary bank", Cardinality: "one", Value: "Chase"},
	{Key: "c.bank.drift", Topic: "bank", Predicate: "primary_bank", Label: "primary bank", Cardinality: "many", Value: "Ally"},
	{Key: "c.barber", Topic: "barber", Predicate: "barber", Label: "barber", Cardinality: "one", Value: "Luis"},
	{Key: "c.barber.second", Topic: "barber", Predicate: "barber", Label: "barber", Cardinality: "one", Value: "Marco"},
	{Key: "c.birthday", Topic: "family", Predicate: "sister_birthday", Label: "sister's birthday", Cardinality: "one", Value: "June 3"},
	{Key: "c.birthday.fixed", Topic: "family", Predicate: "sister_birthday", Label: "sister's birthday", Cardinality: "one", Value: "June 13"},
	{Key: "c.parking", Topic: "parking", Predicate: "parking_spot", Label: "parking spot", Cardinality: "one", Value: "level 2 bay 14"},
	// Corpus v2: Claims whose later updates use different words that still
	// share the saved value or the Predicate's words.
	{Key: "c.carrier", Topic: "carrier", Predicate: "phone_carrier", Label: "phone carrier", Cardinality: "one", Value: "Verizon"},
	{Key: "c.shoe", Topic: "shoe", Predicate: "shoe_size", Label: "shoe size", Cardinality: "one", Value: "9"},
	// Corpus v4: a Claim whose later update has a verb between the change
	// cue and the saved value.
	{Key: "c.storage", Topic: "storage", Predicate: "cloud_storage", Label: "cloud storage", Cardinality: "one", Value: "Dropbox"},
}

func scaleOwner(key, topic, text string) ScaleMessage {
	return ScaleMessage{Key: key, Role: ScaleRoleOwner, Topic: topic, Area: ScaleAreaGlobal, Text: text}
}

func scaleReply(key, topic, text string) *ScaleMessage {
	return &ScaleMessage{Key: key, Role: ScaleRoleAssistant, Topic: topic, Area: ScaleAreaGlobal, Text: text}
}

func scaleRemember(claim, text string) ScaleStep {
	return ScaleStep{Kind: ScaleStepRemember, Claim: claim, Owner: scaleOwner("m."+claim, scaleClaimTopic(claim), text)}
}

func scaleClaimTopic(key string) string {
	for _, claim := range scaleClaims {
		if claim.Key == key {
			return claim.Topic
		}
	}
	return ""
}

func scaleExchange(key, topic, owner, reply string) ScaleStep {
	return ScaleStep{Kind: ScaleStepExchange, Owner: scaleOwner(key, topic, owner), Assistant: scaleReply(key+".a", topic, reply)}
}

// scalePrivateExchange is an exchange labelled private: injecting it into an
// unrelated request counts as a private item.
func scalePrivateExchange(key, topic, owner, reply string) ScaleStep {
	step := scaleExchange(key, topic, owner, reply)
	step.Owner.Private, step.Assistant.Private = true, true
	return step
}

// scaleTopicExchanges are ordinary exchanges on one topic, keyed prefix.1,
// prefix.2, and so on.
func scaleTopicExchanges(prefix, topic string, owners ...string) []ScaleStep {
	steps := make([]ScaleStep, 0, len(owners))
	for i, owner := range owners {
		steps = append(steps, scaleExchange(fmt.Sprintf("%s.%d", prefix, i+1), topic, owner, "Okay, noted."))
	}
	return steps
}

func scaleScenarios() []scaleScenario {
	return []scaleScenario{
		// An ordinary mention that is never linked to the Claim saved later.
		{at: .03, steps: []ScaleStep{scaleExchange("m.parking.casual", "parking", "I finally got a parking spot at the office: level 2, bay 14.", "Nice, that will make mornings easier.")}},
		{at: .04, steps: []ScaleStep{scaleRemember("c.dinner", "Remember that I prefer vegetarian dinners.")}},
		{at: .05, steps: []ScaleStep{scaleRemember("c.parking", "Remember that my parking spot is level 2 bay 14.")}},
		{at: .06, steps: []ScaleStep{scaleRemember("c.coffee", "Remember that my favorite coffee shop is Blue Bottle.")}},
		{at: .08, steps: []ScaleStep{scaleRemember("c.employer", "Remember that I work at Initech.")}},
		{at: .09, steps: []ScaleStep{scaleRemember("c.birthday", "Remember that my sister's birthday is June 3.")}},
		{at: .10, steps: []ScaleStep{scaleRemember("c.home", "Remember that I live in Boston.")}},
		{at: .11, steps: []ScaleStep{scaleRemember("c.gym", "Remember that my gym is Equinox.")}},
		{at: .12, steps: []ScaleStep{scaleRemember("c.dentist", "Remember that my dentist is Dr. Patel.")}},
		{at: .13, steps: []ScaleStep{scaleRemember("c.bank", "Remember that my primary bank is Chase.")}},
		{at: .15, steps: []ScaleStep{scaleRemember("c.barber", "Remember that my barber is Luis.")}},
		{at: .16, steps: []ScaleStep{scaleRemember("c.carrier", "Remember that my phone carrier is Verizon.")}},
		{at: .17, steps: []ScaleStep{scaleRemember("c.shoe", "Remember that my shoe size is 9.")}},
		{at: .45, steps: []ScaleStep{{Kind: ScaleStepRetire, Target: "c.coffee",
			Owner:     scaleOwner("m.retire.coffee", "coffee", "Please retire the saved coffee shop memory; I stopped going there."),
			Assistant: scaleReply("m.retire.coffee.a", "coffee", "Done; that saved coffee shop memory is retired.")}}},
		{at: .50, steps: []ScaleStep{{Kind: ScaleStepRetire, Target: "c.parking",
			Owner:     scaleOwner("m.retire.parking", "parking", "Please retire the saved parking memory; I gave that spot up."),
			Assistant: scaleReply("m.retire.parking.a", "parking", "Done; the parking memory is retired.")}}},
		{at: .55, steps: []ScaleStep{{Kind: ScaleStepCorrect, Claim: "c.employer.new", Target: "c.employer", Mode: "changed",
			Owner: scaleOwner("m.c.employer.new", "employer", "Update my employer: I now work at Globex.")}}},
		{at: .57, steps: []ScaleStep{{Kind: ScaleStepCorrect, Claim: "c.birthday.fixed", Target: "c.birthday", Mode: "error",
			Owner: scaleOwner("m.c.birthday.fixed", "family", "Correction: my sister's birthday is June 13, not June 3.")}}},
		{at: .60, steps: []ScaleStep{scaleExchange("m.moved", "home_city", "I moved to Chicago last month and I'm still unpacking boxes.", "Welcome to Chicago! Want help planning the unpacking?")}},
		{at: .62, steps: []ScaleStep{scaleExchange("m.gym.newer", "gym", "My gym is now the YMCA downtown.", "Got it, the YMCA downtown.")}},
		{at: .65, steps: []ScaleStep{scaleRemember("c.dentist.drift", "Remember that my dentist is Dr. Okafor now.")}},
		{at: .66, steps: []ScaleStep{scaleRemember("c.bank.drift", "Remember that my primary bank is Ally.")}},
		{at: .67, steps: []ScaleStep{scaleRemember("c.barber.second", "Remember that my barber is Marco.")}},
		{at: .75, steps: []ScaleStep{scaleExchange("m.coffee.restated", "coffee", "Honestly, Blue Bottle is still my favorite coffee shop.", "Noted, Blue Bottle it is.")}},
		// Corpus v2. Updates in different words: the saved value with a change
		// cue, and the Predicate's words in another order and inflection.
		{at: .70, steps: []ScaleStep{scaleExchange("m.carrier.switch", "carrier", "I finally dropped Verizon last week and switched to T-Mobile.", "Okay, noted.")}},
		{at: .71, steps: []ScaleStep{scaleExchange("m.shoe.newer", "shoe", "My shoes are a size 10 now after the running season.", "Okay, noted.")}},
		// Distractors that share a word with a saved Claim but do not update or
		// restate it: a value without first person, a value without a change
		// cue, a value with only a novelty word, one of two Predicate words,
		// and a retired value without its Predicate's words.
		{at: .72, steps: []ScaleStep{scaleExchange("m.boston.marathon", "boston", "The Boston marathon moved to a new date this year.", "Okay, noted.")}},
		{at: .73, steps: []ScaleStep{scaleExchange("m.boston.friends", "boston", "My Boston friends are visiting next week.", "Okay, noted.")}},
		{at: .74, steps: []ScaleStep{scaleExchange("m.carrier.bill", "carrier", "Verizon sent me a new bill and I need to check the charges.", "Okay, noted.")}},
		{at: .76, steps: []ScaleStep{scaleExchange("m.shoe.running", "running", "I need new running shoes before the 10k.", "Okay, noted.")}},
		{at: .78, steps: []ScaleStep{scaleExchange("m.coffee.airport", "coffee", "Grabbed a Blue Bottle cold brew at the airport this morning.", "Okay, noted.")}},
		// Corpus v3 (harness review final pass). A real update naming the
		// saved value, followed by newer sentences that share the value and a
		// change cue without updating the Claim: a cue governing another object
		// and a third-party subject. Then third-party mentions of a retired
		// value: a friend's preference and news about the shop.
		{at: .79, steps: []ScaleStep{scaleExchange("m.home.news", "home_city", "Big news: I moved to Chicago last month, Boston is behind me.", "Okay, noted.")}},
		{at: .80, steps: []ScaleStep{scaleExchange("m.boston.umbrella", "boston", "I left my umbrella in Boston.", "Okay, noted.")}},
		{at: .81, steps: []ScaleStep{scaleExchange("m.boston.sister", "boston", "My sister moved to Boston now.", "Okay, noted.")}},
		{at: .82, steps: []ScaleStep{scaleExchange("m.coffee.friend", "coffee", "My friend Sam says his favorite coffee is Blue Bottle.", "Okay, noted.")}},
		{at: .83, steps: []ScaleStep{scaleExchange("m.coffee.closed", "coffee", "The Blue Bottle coffee shop on Main Street closed today.", "Okay, noted.")}},
		// Corpus v4 (confirmation review). An owner update with a verb between
		// the cue and the value; newer updates by someone else and reported
		// speech that share home_city's value and a cue; a third party
		// dropping the saved carrier; and a possessive chain naming someone
		// else's favorite coffee shop.
		{at: .18, steps: []ScaleStep{scaleRemember("c.storage", "Remember that I use Dropbox for cloud storage.")}},
		{at: .795, steps: []ScaleStep{scaleExchange("m.storage.nolonger", "storage", "I no longer use Dropbox.", "Okay, noted.")}},
		{at: .84, steps: []ScaleStep{scaleExchange("m.boston.ex", "boston", "My ex left Boston.", "Okay, noted.")}},
		{at: .845, steps: []ScaleStep{scaleExchange("m.boston.sister_said", "boston", "My sister said, Boston is no longer an option.", "Okay, noted.")}},
		{at: .846, steps: []ScaleStep{scaleExchange("m.carrier.dad", "carrier", "My dad dropped Verizon.", "Okay, noted.")}},
		{at: .847, steps: []ScaleStep{scaleExchange("m.coffee.dad", "coffee", "My dad's favorite coffee shop is Blue Bottle.", "Okay, noted.")}},
		// Corpus v4: private messages whose one rare word an unrelated request
		// shares, among ordinary messages that make every other word of that
		// request known to history (the reviewer's strong-single-match probes).
		{at: .30, steps: append(scaleTopicExchanges("m.engineering", "engineering",
			"Explain how a heat pump works.", "Explain the offside rule.", "The technical rider for the gig is ready.",
			"We need technical support for the printer.", "The engineers fixed the bridge.", "Our engineers are on strike."),
			scalePrivateExchange("m.private.debt", "money", "I owe forty thousand in credit card debt and haven't told Sam.", "That is a lot to carry alone."))},
		{at: .36, steps: append(scaleTopicExchanges("m.newsletter", "newsletter",
			"Write a toast for the wedding.", "Write a cover letter.", "Read me a poem by Mary Oliver.", "This poem is lovely.",
			"The newsletter goes out Friday.", "Fix the newsletter header."),
			scalePrivateExchange("m.private.anxiety", "health", "I've been seeing a therapist about my anxiety since March.", "Thank you for telling me."))},
		{at: .42, steps: append(scaleTopicExchanges("m.playlist", "playlist",
			"Make a playlist for the road trip.", "The playlist needs more jazz.", "The handover meeting is at noon.", "Plan the handover notes."),
			scalePrivateExchange("m.private.custody", "relationship", "My divorce lawyer wants the custody paperwork by Friday.", "That sounds stressful."))},
		{at: .48, steps: append(scaleTopicExchanges("m.vpn", "vpn",
			"The VPN keeps dropping at the cafe.", "The VPN client needs an update.", "Turn the VPN off for the printer.", "The VPN is slow on the hotel network."),
			scalePrivateExchange("m.private.overdrawn", "money", "My bank account is overdrawn by two thousand dollars.", "That sounds stressful."))},
	}
}

// Dense coverage targets pair rare objects and places with synonyms that occur
// nowhere else in the corpus. The lexical generator therefore cannot find
// them; only a dense scan that actually reaches the target vector can.
var scaleDenseObjects = [][2]string{
	{"astrolabe", "stargazer"}, {"barometer", "weatherglass"}, {"sextant", "navigator"}, {"metronome", "ticker"},
	{"hourglass", "sandclock"}, {"kaleidoscope", "prismtube"}, {"periscope", "sightpipe"}, {"harmonica", "mouthharp"},
	{"accordion", "squeezebox"}, {"telescope", "spyglass"}, {"gyroscope", "spinwheel"}, {"abacus", "beadframe"},
	{"theodolite", "surveyscope"}, {"sundial", "shadowclock"}, {"thermos", "vacuumflask"}, {"typewriter", "keystriker"},
	{"gramophone", "phonograph"}, {"lantern", "lamplight"}, {"tambourine", "jinglering"}, {"ukulele", "smallguitar"},
	{"compass", "needlefinder"}, {"stethoscope", "chestlistener"}, {"microscope", "zoomlens"}, {"binoculars", "fieldglasses"},
}

var scaleDensePlaces = [][2]string{
	{"attic", "loft"}, {"cellar", "basement"}, {"pantry", "larder"}, {"wardrobe", "armoire"}, {"shed", "outbuilding"},
	{"sideboard", "credenza"}, {"footlocker", "trunkbox"}, {"cupboard", "cabinetry"},
}

// GenerateScaleCorpus builds a deterministic history and its probes. The same
// options always produce byte-identical output.
func GenerateScaleCorpus(options ScaleCorpusOptions) ScaleCorpus {
	rng := rand.New(rand.NewPCG(options.Seed, 0x6576696573636c65))
	corpus := ScaleCorpus{Version: ScaleCorpusVersion, Options: options, Areas: append([]ScaleArea{{Key: ScaleAreaGlobal, Name: "Global"}}, scaleProjectAreas...)}
	corpus.Claims = append(corpus.Claims, scaleClaims...)
	// Concept groups drive the deterministic test embedder: words in one group
	// share an embedding direction, approximating a paraphrase-aware model.
	corpus.Concepts = map[string][]string{
		"dog": {"dog", "pup", "puppy", "biscuit"}, "annual": {"annual", "yearly"}, "vet": {"vet", "checkup", "appointment"},
		"run": {"run", "jog", "jogging", "running"}, "halfmarathon": {"half", "marathon", "21k"}, "speed": {"pace", "fast"}, "goal": {"goal", "aim", "aiming"},
	}
	counter := 0
	nextKey := func(prefix string) string {
		counter++
		return fmt.Sprintf("%s.%04d", prefix, counter)
	}
	var global []ScaleSession
	byArea := map[string][]ScaleSession{}
	for i := 0; i < options.GlobalSessions; i++ {
		global = append(global, scaleFillerSession(rng, ScaleAreaGlobal, options, nextKey))
	}
	for _, area := range scaleProjectAreas {
		for i := 0; i < options.ProjectSessions; i++ {
			byArea[area.Key] = append(byArea[area.Key], scaleFillerSession(rng, area.Key, options, nextKey))
		}
	}
	// Place needles inside an ordinary session of their own topic and area so
	// they sit among same-topic distractors rather than in isolated sessions.
	for _, needle := range scaleNeedles {
		area := needle.area
		if area == "" {
			area = ScaleAreaGlobal
		}
		sessions := global
		if area != ScaleAreaGlobal {
			sessions = byArea[area]
		}
		index := scaleTopicSession(sessions, min(len(sessions)-1, int(needle.at*float64(len(sessions)))), needle.topic)
		owner := ScaleMessage{Key: needle.key, Role: ScaleRoleOwner, Topic: needle.topic, Area: area, Private: needle.private, Text: needle.owner}
		reply := ScaleMessage{Key: needle.key + ".a", Role: ScaleRoleAssistant, Topic: needle.topic, Area: area, Private: needle.private, Text: needle.reply}
		step := ScaleStep{Kind: ScaleStepExchange, Owner: owner, Assistant: &reply}
		position := rng.IntN(len(sessions[index].Steps) + 1)
		sessions[index].Steps = slicesInsert(sessions[index].Steps, position, step)
	}
	for i := 0; i < options.DenseTargets; i++ {
		object, place := scaleDenseObjects[i%len(scaleDenseObjects)], scaleDensePlaces[i%len(scaleDensePlaces)]
		key := fmt.Sprintf("d.target.%02d", i)
		owner := ScaleMessage{Key: key, Role: ScaleRoleOwner, Topic: "household_items", Area: ScaleAreaGlobal, Text: fmt.Sprintf("The %s is stored in the %s.", object[0], place[0])}
		reply := ScaleMessage{Key: key + ".a", Role: ScaleRoleAssistant, Topic: "household_items", Area: ScaleAreaGlobal, Text: "Okay, noted where it is."}
		index := (i*len(global))/max(1, options.DenseTargets) + rng.IntN(max(1, len(global)/max(1, options.DenseTargets)))
		index = min(index, len(global)-1)
		global[index].Steps = append(global[index].Steps, ScaleStep{Kind: ScaleStepExchange, Owner: owner, Assistant: &reply})
		corpus.Probes = append(corpus.Probes, ScaleProbe{
			ID: fmt.Sprintf("dense.%02d", i), Family: ScaleFamilyDenseCoverage, Path: ScalePathConversationSearch, Area: ScaleAreaGlobal,
			Message: "Where did I leave that thing?", Topics: []string{"household_items"}, Required: []string{key}, Query: object[1] + " " + place[1],
		})
		corpus.Concepts[object[0]] = []string{object[0], object[1]}
		corpus.Concepts[place[0]] = []string{place[0], place[1]}
	}
	// Scenario sessions are separate conversations interleaved in Global time.
	scenarios := scaleScenarios()
	sort.SliceStable(scenarios, func(i, j int) bool { return scenarios[i].at < scenarios[j].at })
	var ordered []ScaleSession
	next := 0
	for i, session := range global {
		for next < len(scenarios) && scenarios[next].at*float64(len(global)) <= float64(i) {
			ordered = append(ordered, scaleScenarioSession(scenarios[next], nextKey))
			next++
		}
		ordered = append(ordered, session)
	}
	for ; next < len(scenarios); next++ {
		ordered = append(ordered, scaleScenarioSession(scenarios[next], nextKey))
	}
	// Interleave project sessions deterministically so Global and project
	// activity alternate the way a working week does.
	areaIndex := map[string]int{}
	var timeline []ScaleSession
	for i, session := range ordered {
		timeline = append(timeline, session)
		for _, area := range scaleProjectAreas {
			sessions := byArea[area.Key]
			due := ((i + 1) * len(sessions)) / len(ordered)
			for areaIndex[area.Key] < due {
				timeline = append(timeline, sessions[areaIndex[area.Key]])
				areaIndex[area.Key]++
			}
		}
	}
	for _, area := range scaleProjectAreas {
		for ; areaIndex[area.Key] < len(byArea[area.Key]); areaIndex[area.Key]++ {
			timeline = append(timeline, byArea[area.Key][areaIndex[area.Key]])
		}
	}
	corpus.Sessions = timeline
	for i := range corpus.Sessions {
		for j := range corpus.Sessions[i].Steps {
			step := &corpus.Sessions[i].Steps[j]
			if step.Kind != ScaleStepExchange {
				step.IdempotencyKey = "idem:v1:" + scaleUUID(rng)
			}
		}
	}
	corpus.Probes = append(scaleProbes(), corpus.Probes...)
	return corpus
}

func scaleScenarioSession(scenario scaleScenario, nextKey func(string) string) ScaleSession {
	return ScaleSession{Key: nextKey("s.scenario"), Area: ScaleAreaGlobal, Steps: append([]ScaleStep(nil), scenario.steps...)}
}

// scaleTopicSession returns the nearest session at or around index whose
// opening topic matches, so a needle lands among same-topic distractors.
func scaleTopicSession(sessions []ScaleSession, index int, topic string) int {
	for distance := 0; distance < len(sessions); distance++ {
		for _, candidate := range []int{index + distance, index - distance} {
			if candidate >= 0 && candidate < len(sessions) && len(sessions[candidate].Steps) > 0 && sessions[candidate].Steps[0].Owner.Topic == topic {
				return candidate
			}
		}
	}
	return index
}

func scaleFillerSession(rng *rand.Rand, area string, options ScaleCorpusOptions, nextKey func(string) string) ScaleSession {
	var topics []scaleTopic
	total := 0
	for _, topic := range scaleTopics {
		if topic.area == area {
			topics = append(topics, topic)
			total += topic.weight
		}
	}
	pick := func() scaleTopic {
		n := rng.IntN(total)
		for _, topic := range topics {
			if n < topic.weight {
				return topic
			}
			n -= topic.weight
		}
		return topics[len(topics)-1]
	}
	session := ScaleSession{Key: nextKey("s"), Area: area}
	topic := pick()
	exchanges := options.MinExchanges + rng.IntN(options.MaxExchanges-options.MinExchanges+1)
	for i := 0; i < exchanges; i++ {
		if i > 0 && rng.IntN(100) < 12 {
			topic = pick() // ordinary mid-session topic drift
		}
		key := nextKey("m")
		text := scaleFill(rng, topic.owner[rng.IntN(len(topic.owner))], topic.slots)
		if rng.IntN(100) < 30 {
			text += scaleAsides[rng.IntN(len(scaleAsides))]
		}
		owner := ScaleMessage{Key: key, Role: ScaleRoleOwner, Topic: topic.key, Area: area, Private: topic.private, Text: text}
		reply := ScaleMessage{Key: key + ".a", Role: ScaleRoleAssistant, Topic: topic.key, Area: area, Private: topic.private, Text: scaleFill(rng, topic.reply[rng.IntN(len(topic.reply))], topic.slots)}
		session.Steps = append(session.Steps, ScaleStep{Kind: ScaleStepExchange, Owner: owner, Assistant: &reply})
		if rng.IntN(100) < 22 {
			key := nextKey("m")
			owner := ScaleMessage{Key: key, Role: ScaleRoleOwner, Topic: "smalltalk", Area: area, Text: scaleLowContent[rng.IntN(len(scaleLowContent))]}
			reply := ScaleMessage{Key: key + ".a", Role: ScaleRoleAssistant, Topic: "smalltalk", Area: area, Text: scaleLowContentReplies[rng.IntN(len(scaleLowContentReplies))]}
			session.Steps = append(session.Steps, ScaleStep{Kind: ScaleStepExchange, Owner: owner, Assistant: &reply})
		}
	}
	return session
}

func scaleFill(rng *rand.Rand, template string, slots map[string][]string) string {
	chosen := map[string]string{}
	var out strings.Builder
	for {
		start := strings.IndexByte(template, '{')
		if start < 0 {
			out.WriteString(template)
			break
		}
		end := strings.IndexByte(template[start:], '}')
		if end < 0 {
			out.WriteString(template)
			break
		}
		out.WriteString(template[:start])
		name := template[start+1 : start+end]
		value, ok := chosen[name]
		if !ok {
			options := slots[name]
			if len(options) == 0 {
				value = name
			} else {
				value = options[rng.IntN(len(options))]
			}
			chosen[name] = value
		}
		out.WriteString(value)
		template = template[start+end+1:]
	}
	text := out.String()
	if text != "" {
		text = strings.ToUpper(text[:1]) + text[1:]
	}
	return text
}

func slicesInsert(steps []ScaleStep, index int, step ScaleStep) []ScaleStep {
	steps = append(steps, ScaleStep{})
	copy(steps[index+1:], steps[index:])
	steps[index] = step
	return steps
}

// scaleUUID returns a canonical random UUIDv4 drawn from the corpus generator.
func scaleUUID(rng *rand.Rand) string {
	var b [16]byte
	for i := range b {
		b[i] = byte(rng.UintN(256))
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// scaleProbes returns the fixed probe set. Topics lists the conversational
// areas that are on-topic for a probe: items from those topics are tolerated
// even when they are not required. Everything else is unwanted.
func scaleProbes() []ScaleProbe {
	type spec struct {
		id, family, area, message, query string
		topics, required                 []string
		checks                           []ScaleStaleCheck
	}
	// Where a model would plausibly phrase the excerpt search differently from
	// the accepted-memory search.
	conversationQueries := map[string]string{"work": "where I work"}
	specs := []spec{
		{"dinner", ScaleFamilyRelevant, ScaleAreaGlobal, "Suggest a dinner for me tonight.", "dinner", []string{"cooking"}, []string{"c.dinner"}, nil},
		{"greenhouse", ScaleFamilyRelevant, ScaleAreaGlobal, "What was the greenhouse trial about again?", "greenhouse trial", []string{"gardening"}, []string{"n.greenhouse"}, nil},
		{"mother", ScaleFamilyRelevant, ScaleAreaGlobal, "What present would my mother like for her birthday?", "mother birthday present", []string{"family"}, []string{"n.mother"}, nil},
		{"tyres", ScaleFamilyRelevant, ScaleAreaGlobal, "What tyre pressure does the car need?", "tyre pressure", []string{"car"}, []string{"n.tyres"}, nil},
		{"pace", ScaleFamilyRelevant, ScaleAreaGlobal, "Remind me of my half-marathon goal pace.", "half-marathon goal pace", []string{"running"}, []string{"n.pace"}, nil},
		{"lisbon", ScaleFamilyRelevant, ScaleAreaGlobal, "When is the Lisbon hotel booked for?", "Lisbon hotel", []string{"travel"}, []string{"n.lisbon"}, nil},
		{"cashew", ScaleFamilyRelevant, ScaleAreaGlobal, "Any recipe ideas that are safe with my allergy?", "allergy recipe", []string{"cooking"}, []string{"n.cashew"}, nil},
		{"vet", ScaleFamilyRelevant, ScaleAreaGlobal, "When is Biscuit due at the vet?", "Biscuit vet", []string{"pets"}, []string{"n.vet"}, nil},
		{"ferritin", ScaleFamilyRelevant, ScaleAreaGlobal, "What did the doctor say about my ferritin?", "ferritin doctor", []string{"health"}, []string{"n.ferritin"}, nil},
		{"flaky", ScaleFamilyRelevant, "project:payments", "Which payments test was flaky?", "flaky test", []string{"payments"}, []string{"n.flaky"}, nil},
		{"revolut", ScaleFamilyRelevant, "project:ledger", "How does the import read Revolut dates?", "Revolut dates import", []string{"ledger"}, []string{"n.revolut"}, nil},
		{"pup", ScaleFamilyParaphrase, ScaleAreaGlobal, "When is my pup's yearly checkup?", "pup yearly checkup", []string{"pets"}, []string{"n.vet"}, nil},
		{"jog", ScaleFamilyParaphrase, ScaleAreaGlobal, "How fast am I aiming to jog the 21k?", "21k speed aim", []string{"running"}, []string{"n.pace"}, nil},
		// M2: the correction question shares words with the old source, which
		// is how a superseded statement resurfaces in ordinary recall.
		{"work", ScaleFamilyStale, ScaleAreaGlobal, "Where do I work these days?", "employer", []string{"employer"}, []string{"c.employer.new"},
			[]ScaleStaleCheck{{Scenario: ScaleStaleCorrection, Issue: "M2", StaleKeys: []string{"m.c.employer", "c.employer"}}}},
		{"birthday", ScaleFamilyStale, ScaleAreaGlobal, "When is my sister's birthday?", "sister birthday", []string{"family"}, []string{"c.birthday.fixed"},
			[]ScaleStaleCheck{{Scenario: ScaleStaleCorrection, Issue: "M2", StaleKeys: []string{"m.c.birthday", "c.birthday"}}}},
		{"parking", ScaleFamilyStale, ScaleAreaGlobal, "Where is my parking spot?", "parking spot", []string{"parking"}, nil,
			[]ScaleStaleCheck{
				{Scenario: ScaleStaleRetiredRepeat, Issue: "M2", StaleKeys: []string{"m.parking.casual"}},
				{Scenario: ScaleStaleRetiredSource, Issue: "M2", Control: true, StaleKeys: []string{"m.c.parking", "c.parking"}},
			}},
		{"coffee", ScaleFamilyStale, ScaleAreaGlobal, "What's my favorite coffee shop?", "favorite coffee shop", []string{"coffee"}, nil,
			[]ScaleStaleCheck{
				{Scenario: ScaleStaleRetiredRestated, Issue: "M2", StaleKeys: []string{"m.coffee.restated"}},
				{Scenario: ScaleStaleRetiredSource, Issue: "M2", Control: true, StaleKeys: []string{"m.c.coffee", "c.coffee"}},
				{Scenario: ScaleStaleOverFlag, Issue: "M2", ClearKeys: []string{"c.coffee", "m.coffee.friend"}},
				{Scenario: ScaleStaleOverFlag, Issue: "M2", ClearKeys: []string{"c.coffee", "m.coffee.closed"}},
				{Scenario: ScaleStaleOverFlag, Issue: "M2", ClearKeys: []string{"c.coffee", "m.coffee.dad"}},
			}},
		// Corpus v2: the retired value itself is the query, so a mention that
		// does not restate the Claim is delivered and its flag can be checked.
		{"blue_bottle", ScaleFamilyStale, ScaleAreaGlobal, "Have I mentioned Blue Bottle before?", "Blue Bottle", []string{"coffee"}, nil,
			[]ScaleStaleCheck{
				{Scenario: ScaleStaleRetiredRestated, Issue: "M2", StaleKeys: []string{"m.coffee.restated"}},
				{Scenario: ScaleStaleOverFlag, Issue: "M2", ClearKeys: []string{"c.coffee", "m.coffee.airport"}},
				{Scenario: ScaleStaleOverFlag, Issue: "M2", ClearKeys: []string{"c.coffee", "m.coffee.friend"}},
				{Scenario: ScaleStaleOverFlag, Issue: "M2", ClearKeys: []string{"c.coffee", "m.coffee.closed"}},
				{Scenario: ScaleStaleOverFlag, Issue: "M2", ClearKeys: []string{"c.coffee", "m.coffee.dad"}},
			}},
		// M3: the question finds the saved claim through its predicate wording,
		// isolating newer-statement detection from claim recall.
		{"home_city", ScaleFamilyStale, ScaleAreaGlobal, "What's my home city these days?", "home city", []string{"home_city"}, []string{"c.home"},
			[]ScaleStaleCheck{
				{Scenario: ScaleStaleNewerWording, Issue: "M3", LinkKeys: []string{"c.home", "m.moved"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.home", "m.boston.marathon"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.home", "m.boston.friends"}},
				// Corpus v3: the real update must not be crowded out by newer
				// sentences that share the value and a cue without updating it.
				{Scenario: ScaleStaleNewerSavedValue, Issue: "M3", LinkKeys: []string{"c.home", "m.home.news"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.home", "m.boston.umbrella"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.home", "m.boston.sister"}},
				// Corpus v4: newer third-party and reported updates must not
				// take the owner's update's companion slot.
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.home", "m.boston.ex"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.home", "m.boston.sister_said"}},
			}},
		// Corpus v2: the newer statement shares the saved value or the
		// Predicate's words, but not the Predicate phrase.
		{"carrier", ScaleFamilyStale, ScaleAreaGlobal, "Which phone carrier am I with?", "phone carrier", []string{"carrier"}, []string{"c.carrier"},
			[]ScaleStaleCheck{
				{Scenario: ScaleStaleNewerSavedValue, Issue: "M3", LinkKeys: []string{"c.carrier", "m.carrier.switch"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.carrier", "m.carrier.bill"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.carrier", "m.carrier.dad"}},
			}},
		{"shoe_size", ScaleFamilyStale, ScaleAreaGlobal, "What's my shoe size?", "shoe size", []string{"shoe"}, []string{"c.shoe"},
			[]ScaleStaleCheck{
				{Scenario: ScaleStaleNewerPredicate, Issue: "M3", LinkKeys: []string{"c.shoe", "m.shoe.newer"}},
				{Scenario: ScaleStaleOverLink, Issue: "M3", ClearKeys: []string{"c.shoe", "m.shoe.running"}},
			}},
		// Corpus v4: an owner update with a verb between the change cue and the
		// saved value ("I no longer use Dropbox.").
		{"storage", ScaleFamilyStale, ScaleAreaGlobal, "Which cloud storage do I use?", "cloud storage", []string{"storage"}, []string{"c.storage"},
			[]ScaleStaleCheck{{Scenario: ScaleStaleNewerSavedValue, Issue: "M3", LinkKeys: []string{"c.storage", "m.storage.nolonger"}}}},
		{"gym", ScaleFamilyStale, ScaleAreaGlobal, "Which gym do I go to?", "gym", []string{"gym"}, []string{"c.gym"},
			[]ScaleStaleCheck{{Scenario: ScaleStaleNewerSameWording, Issue: "M3", Control: true, LinkKeys: []string{"c.gym", "m.gym.newer"}}}},
		{"dentist", ScaleFamilyStale, ScaleAreaGlobal, "Who is my dentist?", "dentist", []string{"dentist"}, []string{"c.dentist", "c.dentist.drift"},
			[]ScaleStaleCheck{{Scenario: ScaleStaleLabelDrift, Issue: "M4", LinkKeys: []string{"c.dentist", "c.dentist.drift"}}}},
		{"bank", ScaleFamilyStale, ScaleAreaGlobal, "Which bank is my primary bank?", "primary bank", []string{"bank"}, []string{"c.bank", "c.bank.drift"},
			[]ScaleStaleCheck{{Scenario: ScaleStaleCardinalityDrift, Issue: "M4", LinkKeys: []string{"c.bank", "c.bank.drift"}}}},
		{"barber", ScaleFamilyStale, ScaleAreaGlobal, "Who is my barber?", "barber", []string{"barber"}, []string{"c.barber", "c.barber.second"},
			[]ScaleStaleCheck{{Scenario: ScaleStaleSamePredicate, Issue: "M4", Control: true, LinkKeys: []string{"c.barber", "c.barber.second"}}}},
	}
	var probes []ScaleProbe
	for _, s := range specs {
		probes = append(probes, ScaleProbe{ID: "auto." + s.id, Family: s.family, Path: ScalePathAutomatic, Area: s.area, Message: s.message, Topics: s.topics, Required: s.required, Checks: s.checks})
	}
	automaticOnly := []ScaleProbe{
		{ID: "auto.follow.basil", Family: ScaleFamilyFollowUp, Area: ScaleAreaGlobal, Prelude: []string{"How should I prune the tomato plants this weekend?"}, Message: "and the basil?", Topics: []string{"gardening"}, Required: []string{"n.basil"}},
		{ID: "auto.follow.dates", Family: ScaleFamilyFollowUp, Area: ScaleAreaGlobal, Prelude: []string{"Let's plan the Lisbon trip."}, Message: "what dates did I book?", Topics: []string{"travel"}, Required: []string{"n.lisbon"}},
		// Low-content follow-ups add no new information need. Per the M1 default
		// (injected items must match a distinctive current-message term), every
		// injected item is unwanted, including items about the prelude topic.
		{ID: "auto.low.thanks", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"How should I prune the tomato plants this weekend?"}, Message: "thanks!"},
		{ID: "auto.low.ok", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"Draft a packing list for the Porto trip."}, Message: "ok"},
		{ID: "auto.low.sounds_good", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"Plan an interval run for Thursday morning."}, Message: "sounds good, thanks"},
		{ID: "auto.low.perfect", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"What should I get my sister for her birthday?"}, Message: "perfect"},
		{ID: "auto.low.project", Family: ScaleFamilyLowContent, Area: "project:payments", Prelude: []string{"Why did the deploy to staging fail this morning?"}, Message: "great, thanks!"},
		{ID: "auto.low.fresh", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Message: "thanks!"},
		// Privacy: unrelated requests sharing a word with private history. The
		// project control shows scope isolation holds where relevance does not.
		{ID: "auto.privacy.ci", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "Can you summarize the test results from today's CI run for the payments service?", Topics: []string{"payments"}},
		{ID: "auto.privacy.orm", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "How do I model a many-to-many relationship in the ORM?", Topics: []string{"ledger", "payments"}},
		{ID: "auto.privacy.project", Family: ScaleFamilyPrivacy, Area: "project:payments", Message: "Summarize the test results from the last CI run.", Topics: []string{"payments"}},
		{ID: "auto.unrelated.math", Family: ScaleFamilyUnrelated, Area: ScaleAreaGlobal, Message: "What's 17 times 23?"},
		{ID: "auto.unrelated.translate", Family: ScaleFamilyUnrelated, Area: ScaleAreaGlobal, Message: "Translate good morning into Portuguese."},
		{ID: "auto.unrelated.haiku", Family: ScaleFamilyUnrelated, Area: ScaleAreaGlobal, Message: "Write a haiku about autumn."},
		// Corpus v3 (harness review final pass). Ordinary questions with
		// conversational filler or a word history never used, whose answer
		// shares one content word with them.
		{ID: "auto.one.passport", Family: ScaleFamilyOneWordAnswer, Area: ScaleAreaGlobal, Message: "When do I need to renew my passport?", Topics: []string{"travel"}, Required: []string{"n.passport"}},
		{ID: "auto.one.parked", Family: ScaleFamilyOneWordAnswer, Area: ScaleAreaGlobal, Message: "Do you know where I parked the car?", Topics: []string{"car", "parking"}, Required: []string{"n.parked"}},
		{ID: "auto.one.sister", Family: ScaleFamilyOneWordAnswer, Area: ScaleAreaGlobal, Message: "What's my sister's birthday again?", Topics: []string{"family"}, Required: []string{"n.sister"}},
		{ID: "auto.one.cat_vet", Family: ScaleFamilyOneWordAnswer, Area: ScaleAreaGlobal, Message: "Remind me which vet we use for the cat", Topics: []string{"pets"}, Required: []string{"n.cat_vet"}},
		{ID: "auto.one.wifi", Family: ScaleFamilyOneWordAnswer, Area: ScaleAreaGlobal, Message: "ok so what's my wifi password", Topics: []string{"home"}, Required: []string{"n.wifi"}},
		// Acknowledgements and commands about the live context ("it").
		{ID: "auto.low.got_it", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"How should I prune the tomato plants this weekend?"}, Message: "got it, thanks"},
		{ID: "auto.low.thats_it", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"Draft a packing list for the Porto trip."}, Message: "that's it, thanks"},
		{ID: "auto.low.love_it", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"Plan an interval run for Thursday morning."}, Message: "love it"},
		{ID: "auto.low.do_it", Family: ScaleFamilyLowContent, Area: ScaleAreaGlobal, Prelude: []string{"Book the car service before the 60,000 km check."}, Message: "ok do it"},
		// A referring question that ties on every earlier root refers to the
		// most recent topic.
		{ID: "auto.follow.when", Family: ScaleFamilyFollowUp, Area: ScaleAreaGlobal, Prelude: []string{"How should I prune the tomato plants this weekend?", "Which tyre brand lasts longest?", "Let's plan the Lisbon trip."}, Message: "and when was it?", Topics: []string{"travel"}, Required: []string{"n.lisbon"}},
		// A request mostly about something history never discussed, sharing
		// its one rare known word with private history.
		{ID: "auto.privacy.energy", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "What's the energy rating of the dishwasher?", Topics: []string{"home"}},
		// Corpus v4 (confirmation review): requests that are not about
		// recalling the owner, whose words history all knows and whose rarest
		// word is shared with one private message.
		{ID: "auto.privacy.debt", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "Explain technical debt to the new engineers.", Topics: []string{"engineering"}},
		{ID: "auto.privacy.anxiety", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "Write a short poem about anxiety for my newsletter.", Topics: []string{"newsletter"}},
		{ID: "auto.privacy.custody", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "Suggest a playlist for a custody handover drive.", Topics: []string{"playlist"}},
		{ID: "auto.privacy.run", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "What were the results of the run today?", Topics: []string{"running"}},
		{ID: "auto.privacy.vpn", Family: ScaleFamilyPrivacy, Area: ScaleAreaGlobal, Message: "Do I need to use a VPN for the bank?", Topics: []string{"vpn"}},
	}
	for _, probe := range automaticOnly {
		probe.Path = ScalePathAutomatic
		probes = append(probes, probe)
	}
	// Each read tool is measured on its own with the query a model would plausibly
	// choose. Required items are split by evidence kind: Claims for
	// memory_search, original messages for conversation search. Link checks,
	// and checks that a statement is not linked, need accepted Claims, which
	// conversation search never returns.
	for _, s := range specs {
		for _, path := range []string{ScalePathMemorySearch, ScalePathConversationSearch} {
			probe := ScaleProbe{ID: path + "." + s.id, Family: s.family, Path: path, Area: s.area, Message: "Look this up in my memory: " + s.message, Query: s.query, Topics: s.topics}
			if query := conversationQueries[s.id]; path == ScalePathConversationSearch && query != "" {
				probe.Query = query
			}
			for _, key := range s.required {
				if strings.HasPrefix(key, "c.") == (path == ScalePathMemorySearch) {
					probe.Required = append(probe.Required, key)
				}
			}
			for _, check := range s.checks {
				if len(check.LinkKeys) == 0 && check.Scenario != ScaleStaleOverLink || path == ScalePathMemorySearch {
					probe.Checks = append(probe.Checks, check)
				}
			}
			probes = append(probes, probe)
		}
	}
	return probes
}
