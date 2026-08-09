// Full-file approval preview. Each pane is a handful of text blocks rather
// than one React node per line, so the 100KB file limit remains responsive.

import {
  fullFileDiff,
  newFilePane,
  type DiffPane,
  type DiffSection,
} from "./fileDiff";

type Props = { oldText: string; newText: string; isNew?: boolean };

export function Diff({ oldText, newText, isNew = false }: Props) {
  if (isNew) {
    return (
      <Frame single>
        <Pane label="New file" pane={newFilePane(newText)} />
      </Frame>
    );
  }

  const diff = fullFileDiff(oldText, newText);
  return (
    <Frame>
      <Pane label="Before" pane={diff.before} />
      <Pane label="After" pane={diff.after} bordered />
    </Frame>
  );
}

function Frame({ children, single = false }: { children: React.ReactNode; single?: boolean }) {
  return (
    <section
      aria-label="Complete file change preview"
      className="border-hair-strong bg-code overflow-x-auto border-y font-mono text-[11.5px] leading-[1.65]"
    >
      <div className={`${single ? "min-w-[460px]" : "grid min-w-[820px] grid-cols-2"}`}>
        {children}
      </div>
    </section>
  );
}

function Pane({
  label,
  pane,
  bordered = false,
}: {
  label: string;
  pane: DiffPane;
  bordered?: boolean;
}) {
  return (
    <section
      aria-label={`${label} file version`}
      className={bordered ? "border-hair-strong min-w-0 border-l" : "min-w-0"}
    >
      <div className="border-hair-strong text-fainter border-b px-3 py-[7px] font-sans text-[10.5px] font-semibold uppercase tracking-[.08em]">
        {label}
      </div>
      <div className="overflow-x-auto">
        <div className="min-w-max">
          {pane.sections.length === 0 ? (
            <div className="text-fainter px-3 py-4">Empty file</div>
          ) : (
            pane.sections.map((section) => (
              <Section key={`${section.startLine}-${section.kind}`} section={section} />
            ))
          )}
          {pane.missingFinalNewline && (
            <div role="note" className="text-amber-ink px-3 py-1 text-[10.5px] italic">
              No newline at end of file
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function Section({ section }: { section: DiffSection }) {
  if (section.kind === "spacer") {
    const blank = Array.from({ length: section.lines.length }, () => " ").join("\n");
    return (
      <div aria-hidden="true" className="flex bg-[rgba(255,255,255,.012)]">
        <pre className="m-0 flex-none py-0 pr-[10px] pl-2">{blank}</pre>
        <pre className="m-0 pr-3">{blank}</pre>
      </div>
    );
  }

  const sign = section.kind === "removed" ? "-" : section.kind === "added" ? "+" : " ";
  const endLine = section.startLine + section.lines.length - 1;
  const tone =
    section.kind === "removed"
      ? "bg-[rgba(217,107,107,.08)] text-danger-ink"
      : section.kind === "added"
        ? "bg-[rgba(95,174,125,.09)] text-ok-ink"
        : "text-muted-text";
  const gutter =
    section.kind === "removed"
      ? "text-danger-gutter"
      : section.kind === "added"
        ? "text-ok-gutter"
        : "text-ghost";
  const label =
    section.kind === "same"
      ? `Unchanged lines ${section.startLine} to ${endLine}`
      : `${section.kind === "removed" ? "Removed" : "Added"} lines ${section.startLine} to ${endLine}`;

  return (
    <div aria-label={label} className={`${tone} flex`}>
      <pre className={`${gutter} m-0 flex-none py-0 pr-[10px] pl-2 text-right select-none`}>
        {section.lines
          .map((_, index) => `${sign} ${section.startLine + index}`)
          .join("\n")}
      </pre>
      <pre className="m-0 pr-3 whitespace-pre">
        {section.lines.map((line) => (line === "" ? " " : line)).join("\n")}
      </pre>
    </div>
  );
}
