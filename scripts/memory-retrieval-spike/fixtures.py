"""Versioned synthetic #165 inputs; unrelated to the #167/#168 release holdout.

Each family, its lexical control, its paraphrase, and its forbidden-scope twin
belong to exactly one partition. Held-out queries are never used by development.
"""
import json
from pathlib import Path

DEVELOPMENT = [
    ("I eat only plants and avoid all animal products.", "Which meals fit my vegan diet?", "animal products"),
    ("I ride a bicycle to the office every morning.", "How do I commute to work without a car?", "bicycle office"),
    ("My grandmother cannot hear quiet voices clearly.", "What hearing accommodation does my grandma need?", "grandmother voices"),
    ("The apartment keys are inside the blue ceramic bowl.", "Where did I leave the spare door key?", "keys ceramic"),
    ("I turn off phone alerts during the hours I am asleep.", "What is my nighttime notification preference?", "phone alerts"),
    ("The server keeps a spare copy of every file on another disk.", "How are our documents protected against drive failure?", "spare disk"),
    ("I cannot digest dairy sugar and choose milk alternatives.", "Why do I need lactose-free food?", "dairy alternatives"),
    ("We planned to gather rainwater in barrels for watering plants.", "What was our idea for irrigating the garden sustainably?", "rainwater barrels"),
    ("My nephew becomes sick when riding in the back of a car.", "How does road travel affect my young relative?", "nephew car"),
    ("I prefer to pay the entire card balance before interest accrues.", "How do I avoid financing charges on credit purchases?", "card balance"),
    ("The dog hides under furniture when fireworks explode.", "What scares my pet during loud celebrations?", "dog fireworks"),
    ("The museum opens late on Thursday evenings.", "When can I view the exhibits after normal business hours?", "museum Thursday"),
    ("I store flour in sealed containers to keep insects away.", "How do I prevent pantry pests from reaching baking supplies?", "flour insects"),
    ("Our deployment waits for a human to authorize the release.", "What approval is needed before software goes live?", "deployment authorize"),
    ("I requested a room without stairs because my ankle is injured.", "Why do I need accessible lodging on the ground floor?", "stairs ankle"),
    ("I switch the display to amber after sunset.", "What screen color do I use in the evening?", "display amber"),
]

HELDOUT = [
    ("I bring earplugs because the train wheels squeal painfully.", "How do I protect my hearing on rail journeys?", "earplugs train"),
    ("My sister keeps a written list of ingredients that cause allergic reactions.", "Where does my sibling track foods she must avoid?", "sister ingredients"),
    ("The roof panels convert sunlight into electricity for our house.", "What renewable energy powers my home?", "roof sunlight"),
    ("I saved the receipt so the defective kettle can be returned.", "What proof of purchase do I need for the broken appliance refund?", "receipt kettle"),
    ("We postponed the picnic after the forecast predicted heavy showers.", "Why was our outdoor lunch delayed?", "picnic showers"),
    ("I water the orchid only after its potting mix becomes dry.", "How do I decide whether my flowering houseplant needs a drink?", "orchid dry"),
    ("My father uses a magnifying lens to read small print.", "What helps my dad see tiny letters?", "father magnifying"),
    ("The team copies the production database before changing its schema.", "What safeguard precedes a data migration?", "database schema"),
    ("I chose a seat next to the aisle to stretch my legs.", "Why do I prefer sitting beside the walkway on flights?", "seat aisle"),
    ("The grocery delivery must arrive before the frozen food thaws.", "What timing constraint protects the cold chain for my shopping?", "grocery frozen"),
    ("I practice slow breathing when presentations make me nervous.", "How do I calm anxiety before public speaking?", "breathing presentations"),
    ("We replace the smoke alarm battery when the warning chirp begins.", "What maintenance follows the fire detector's low-power sound?", "smoke battery"),
    ("My winter boots have deep rubber grooves to avoid sliding on ice.", "What keeps my footwear from slipping on frozen pavement?", "boots grooves"),
    ("I marked the document confidential because it includes employee salaries.", "Why does that file require restricted access?", "confidential salaries"),
    ("The child borrows books from the neighborhood library every Saturday.", "Where does the youngster get weekend reading material?", "child library"),
    ("I keep the emergency flashlight in the drawer beside the bed.", "Where can I find a light during a nighttime power failure?", "flashlight drawer"),
]

def write_fixtures(directory: Path):
    directory.mkdir(parents=True, exist_ok=True)
    scopes = ["global", "general", "workspace", "project_a", "project_b"]
    corpus, partitions = [], {}
    for partition, families in [("development", DEVELOPMENT), ("heldout", HELDOUT)]:
        queries = []
        for i, (text, paraphrase, lexical) in enumerate(families):
            stem = f"{partition}-{i:02d}"
            scope = scopes[i % len(scopes)]
            kind = "claim" if i % 2 == 0 else "conversation"
            evidence_kind = "accepted_memory" if kind == "claim" else "conversation_excerpt"
            suffix = "/claim" if kind == "claim" else "/excerpt"
            corpus.append({"ID": stem, "Scope": scope, "Kind": kind, "Text": text})
            forbidden = "project_b" if scope == "project_a" else "project_a"
            corpus.append({"ID": stem + "-forbidden", "Scope": forbidden, "Kind": kind, "Text": text})
            for category, query in [("paraphrase", paraphrase), ("lexical_control", lexical)]:
                queries.append({"id": stem + "-" + category, "family": stem, "scope": scope,
                                "kind": evidence_kind, "query": query, "expected": [stem + suffix],
                                "forbidden": [stem + "-forbidden" + suffix], "category": category})
        partitions[partition] = queries
    objects = ["calendar", "notebook", "stapler", "camera", "backpack", "lamp", "pencil", "envelope", "folder", "clock", "cable", "basket", "table", "vase", "towel", "curtain"]
    colors = ["red", "orange", "yellow", "green", "blue", "violet", "silver", "brown", "black", "white"]
    for i in range(320):
        corpus.append({"ID": f"distractor-{i:03d}", "Scope": scopes[i % 5],
                       "Kind": "claim" if i % 2 == 0 else "conversation",
                       "Text": f"The {colors[(i // 16) % 10]} {objects[i % 16]} in storage bin {i} was catalogued during the inventory."})
    for scope in scopes:
        corpus.append({"ID": "secret-" + scope, "Scope": scope, "Kind": "conversation",
                       "Text": "The synthetic inventory credential is password=DO-NOT-EMBED-THIS-SYNTHETIC-SECRET."})
    (directory / "corpus.json").write_text(json.dumps(corpus, indent=2) + "\n")
    for partition, queries in partitions.items():
        (directory / (partition + ".json")).write_text(json.dumps(queries, indent=2) + "\n")

if __name__ == "__main__":
    import sys
    write_fixtures(Path(sys.argv[1]))
