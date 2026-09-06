from pathlib import Path

from pypdf import PdfReader, PdfWriter, Transformation
from reportlab.lib.colors import HexColor
from reportlab.lib.pagesizes import A3, landscape
from reportlab.pdfgen.canvas import Canvas


ROOT = Path("/Users/davidboktor/code/evie")
WORK = ROOT / "tmp" / "pdfs"
VECTOR = WORK / "vector"
BASE = WORK / "evie-memory-atlas-base.pdf"
OUTPUT = ROOT / "output" / "pdf" / "evie-memory-architecture-atlas-stages-1-3.pdf"

PAGE_WIDTH, PAGE_HEIGHT = landscape(A3)
NAVY = HexColor("#17324d")
BLUE = HexColor("#245b87")
PALE_BLUE = HexColor("#e8f1f8")
GREEN = HexColor("#4e8467")
PALE_GREEN = HexColor("#edf5ef")
GOLD = HexColor("#a4843c")
PALE_GOLD = HexColor("#f7f0df")
INK = HexColor("#24313d")
MUTED = HexColor("#65717c")
LINE = HexColor("#d9dee2")
PAPER = HexColor("#fbfcfd")
WHITE = HexColor("#ffffff")


PAGES = [
    {
        "file": "01-system-context.pdf",
        "section": "C4 - SYSTEM CONTEXT",
        "title": "Memory System Context",
        "subtitle": "Evie is the local authority boundary; OpenRouter receives only approved, bounded conversational requests.",
        "source": "Sources: cmd/evie/main.go | internal/agent/agent.go | internal/openrouter",
    },
    {
        "file": "02-runtime-containers.pdf",
        "section": "C4 - CONTAINERS",
        "title": "Runtime and Persistence Boundaries",
        "subtitle": "The Go runtime owns behavior, SQLite owns durable truth, and the React UI remains an adapter over scoped APIs.",
        "source": "Sources: cmd/evie | internal/web | internal/eviedb/db.go",
    },
    {
        "file": "03-runtime-components.pdf",
        "section": "C4 - COMPONENTS",
        "title": "Go Package Ownership",
        "subtitle": "Deep Kernel-owned seams keep frontends, plugins, and model-facing tools away from raw memory tables and authority rules.",
        "source": "Sources: internal/agent | internal/memory | internal/eviedb | internal/tools",
    },
    {
        "file": "04-stage-evolution.pdf",
        "section": "MEMORY MODEL",
        "title": "Stages 1-3 and Their Sources of Truth",
        "subtitle": "Episodic evidence, working context, and accepted semantic state answer different questions and must never collapse into one store.",
        "source": "Sources: cmd/evie/docs/active/memory.spec.md | memory.decisions.md",
    },
    {
        "file": "05-turn-context-flow.pdf",
        "section": "STAGES 1-2 - DYNAMIC VIEW",
        "title": "Fenced Turn and Working Context Flow",
        "subtitle": "Every remote call is reconstructed from accepted events, bounded, snapshotted, and protected by durable turn ownership.",
        "source": "Sources: internal/agent/agent.go | context.go | compaction.go | internal/eviedb/events.go",
    },
    {
        "file": "06-semantic-write-path.pdf",
        "section": "STAGE 3 - DYNAMIC VIEW",
        "title": "Canonical Semantic Mutation Path",
        "subtitle": "Prepare, preview, approve, and atomically apply one exact compound operation against pinned scope revisions.",
        "source": "Sources: internal/agent/semantic_memory.go | internal/eviedb/semantic*.go",
    },
    {
        "file": "07-semantic-domain-model.pdf",
        "section": "STAGE 3 - DOMAIN MODEL",
        "title": "Semantic Memory Domain",
        "subtitle": "Accepted operations are canonical history; the temporal property graph and cached current state are deterministic projections.",
        "source": "Sources: internal/memory/semantic.go | internal/eviedb/semantic.go",
    },
    {
        "file": "08-scope-promotion.pdf",
        "section": "STAGE 3 - AUTHORITY VIEW",
        "title": "Scope Isolation and Explicit Promotion",
        "subtitle": "A session reads one exact scope set; broader reuse creates mapped objects through an approved promotion rather than changing scope in place.",
        "source": "Sources: internal/memory/scope.go | semantic_scope.go | semantic_promotion.go",
    },
    {
        "file": "09-temporal-lifecycle.pdf",
        "section": "STAGE 3 - TEMPORAL VIEW",
        "title": "Bitemporal Corrections and Reversible Lifecycle",
        "subtitle": "Valid time describes the world; transaction time and Scope Revision describe what Evie accepted and when.",
        "source": "Sources: internal/eviedb/semantic_correction.go | semantic_lifecycle.go",
    },
    {
        "file": "10-replay-recovery.pdf",
        "section": "STAGE 3 - RECOVERY VIEW",
        "title": "Replay, Verification, Quarantine, and Rebuild",
        "subtitle": "The operation stream recreates a shadow projection; only a completely verified result can replace live query state.",
        "source": "Sources: internal/eviedb/semantic_replay.go | semantic_evaluation_test.go",
    },
]


def draw_cover(canvas: Canvas) -> None:
    canvas.setFillColor(NAVY)
    canvas.rect(0, 0, PAGE_WIDTH, PAGE_HEIGHT, stroke=0, fill=1)
    canvas.setFillColor(BLUE)
    canvas.rect(0, PAGE_HEIGHT - 18, PAGE_WIDTH, 18, stroke=0, fill=1)
    canvas.setFillColor(GREEN)
    canvas.rect(0, 0, PAGE_WIDTH * 0.34, 10, stroke=0, fill=1)
    canvas.setFillColor(GOLD)
    canvas.rect(PAGE_WIDTH * 0.34, 0, PAGE_WIDTH * 0.22, 10, stroke=0, fill=1)
    canvas.setFillColor(BLUE)
    canvas.rect(PAGE_WIDTH * 0.56, 0, PAGE_WIDTH * 0.44, 10, stroke=0, fill=1)

    canvas.setFillColor(PALE_BLUE)
    canvas.setFont("Helvetica", 12)
    canvas.drawString(64, PAGE_HEIGHT - 72, "EVIE ARCHITECTURE REFERENCE")
    canvas.setFillColor(WHITE)
    canvas.setFont("Helvetica-Bold", 36)
    canvas.drawString(64, PAGE_HEIGHT - 132, "Memory Architecture Atlas")
    canvas.setFont("Helvetica", 23)
    canvas.drawString(64, PAGE_HEIGHT - 170, "Implemented Stages 1-3")

    canvas.setStrokeColor(HexColor("#54718a"))
    canvas.setLineWidth(1)
    canvas.line(64, PAGE_HEIGHT - 202, PAGE_WIDTH - 64, PAGE_HEIGHT - 202)

    cards = [
        ("01", "EPISODIC MEMORY", "Immutable scoped events preserve what happened.", PALE_BLUE, BLUE),
        ("02", "WORKING MEMORY", "A bounded request projection controls what the model sees.", PALE_GREEN, GREEN),
        ("03", "SEMANTIC MEMORY", "Accepted operations produce a temporal, sourced graph.", PALE_GOLD, GOLD),
    ]
    card_y = PAGE_HEIGHT - 315
    card_gap = 18
    card_width = (PAGE_WIDTH - 128 - card_gap * 2) / 3
    for index, (number, heading, text, fill, accent) in enumerate(cards):
        x = 64 + index * (card_width + card_gap)
        canvas.setFillColor(fill)
        canvas.roundRect(x, card_y, card_width, 82, 7, stroke=0, fill=1)
        canvas.setFillColor(accent)
        canvas.setFont("Helvetica-Bold", 18)
        canvas.drawString(x + 18, card_y + 49, number)
        canvas.setFillColor(INK)
        canvas.setFont("Helvetica-Bold", 10)
        canvas.drawString(x + 52, card_y + 53, heading)
        canvas.setFont("Helvetica", 9)
        canvas.drawString(x + 18, card_y + 24, text)

    canvas.setFillColor(WHITE)
    canvas.setFont("Helvetica-Bold", 12)
    canvas.drawString(64, card_y - 45, "VIEWS")
    contents = [
        "01  System context",
        "02  Runtime containers",
        "03  Go package ownership",
        "04  Stage evolution and authorities",
        "05  Fenced turn and context flow",
        "06  Semantic mutation path",
        "07  Semantic domain model",
        "08  Scope and promotion",
        "09  Bitemporal lifecycle",
        "10  Replay and recovery",
    ]
    for i, item in enumerate(contents):
        column = 0 if i < 5 else 1
        row = i if i < 5 else i - 5
        x = 64 + column * 360
        y = card_y - 75 - row * 25
        canvas.setFillColor(HexColor("#dbe5ed"))
        canvas.setFont("Helvetica", 10)
        canvas.drawString(x, y, item)

    canvas.setFillColor(HexColor("#9fb2c1"))
    canvas.setFont("Helvetica", 8.5)
    canvas.drawString(64, 38, "Architecture Reference v1.0 | Implementation baseline 18c02e6 | 2026-09-03")
    canvas.drawRightString(PAGE_WIDTH - 64, 38, "Stages 4+ are deliberately excluded")


def draw_diagram_page(canvas: Canvas, page: dict, page_number: int) -> None:
    canvas.setFillColor(PAPER)
    canvas.rect(0, 0, PAGE_WIDTH, PAGE_HEIGHT, stroke=0, fill=1)
    canvas.setFillColor(NAVY)
    canvas.rect(0, PAGE_HEIGHT - 14, PAGE_WIDTH, 14, stroke=0, fill=1)

    canvas.setFillColor(BLUE)
    canvas.roundRect(44, PAGE_HEIGHT - 55, 178, 20, 5, stroke=0, fill=1)
    canvas.setFillColor(WHITE)
    canvas.setFont("Helvetica-Bold", 8)
    canvas.drawCentredString(133, PAGE_HEIGHT - 49, page["section"])

    canvas.setFillColor(INK)
    canvas.setFont("Helvetica-Bold", 23)
    canvas.drawString(44, PAGE_HEIGHT - 84, page["title"])
    canvas.setFillColor(MUTED)
    canvas.setFont("Helvetica", 10)
    canvas.drawString(44, PAGE_HEIGHT - 104, page["subtitle"])

    canvas.setStrokeColor(LINE)
    canvas.setLineWidth(0.7)
    canvas.line(44, PAGE_HEIGHT - 119, PAGE_WIDTH - 44, PAGE_HEIGHT - 119)
    canvas.line(44, 37, PAGE_WIDTH - 44, 37)

    canvas.setFillColor(MUTED)
    canvas.setFont("Helvetica", 7.5)
    canvas.drawString(44, 23, page["source"])
    canvas.drawRightString(PAGE_WIDTH - 44, 23, f"EVIE MEMORY | {page_number:02d}")


def create_base_pdf() -> None:
    canvas = Canvas(str(BASE), pagesize=(PAGE_WIDTH, PAGE_HEIGHT))
    canvas.setTitle("Evie Memory Architecture Atlas - Stages 1-3")
    canvas.setAuthor("Evie")
    canvas.setSubject("Official architecture views for implemented Memory Stages 1-3")
    draw_cover(canvas)
    canvas.showPage()
    for index, page in enumerate(PAGES, start=1):
        draw_diagram_page(canvas, page, index)
        canvas.showPage()
    canvas.save()


def merge_diagrams() -> None:
    base_reader = PdfReader(str(BASE))
    writer = PdfWriter()
    writer.add_page(base_reader.pages[0])

    content_x = 50
    content_y = 48
    content_width = PAGE_WIDTH - 100
    content_height = PAGE_HEIGHT - 184

    for index, page_info in enumerate(PAGES, start=1):
        page = base_reader.pages[index]
        diagram_path = VECTOR / page_info["file"]
        diagram = PdfReader(str(diagram_path)).pages[0]
        diagram_width = float(diagram.mediabox.width)
        diagram_height = float(diagram.mediabox.height)
        scale = min(content_width / diagram_width, content_height / diagram_height)
        rendered_width = diagram_width * scale
        rendered_height = diagram_height * scale
        x = content_x + (content_width - rendered_width) / 2
        y = content_y + (content_height - rendered_height) / 2
        page.merge_transformed_page(
            diagram,
            Transformation().scale(scale).translate(x, y),
            expand=False,
            over=True,
        )
        writer.add_page(page)

    metadata = {
        "/Title": "Evie Memory Architecture Atlas - Stages 1-3",
        "/Author": "Evie",
        "/Subject": "Architecture reference for Episodic, Working, and Semantic Memory",
        "/Keywords": "Evie, memory, C4, episodic, working context, semantic memory, SQLite",
    }
    writer.add_metadata(metadata)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with OUTPUT.open("wb") as stream:
        writer.write(stream)


if __name__ == "__main__":
    create_base_pdf()
    merge_diagrams()
    print(OUTPUT)
