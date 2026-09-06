import type { ReactNode } from "react";
import type { SemanticObjectInspection } from "../api/memory";
import { Memory } from "../memory/Memory";
import { Database } from "./Database";

export type DataSource = "database" | "memory";

const sources: { id: DataSource; label: string; description: string }[] = [
  { id: "database", label: "Database", description: "Physical schema and approved records" },
  { id: "memory", label: "Memory", description: "Scoped accepted knowledge" },
];

export function DataHub({
  source,
  onSource,
  onOpenMemoryDetail,
}: {
  source: DataSource;
  onSource: (source: DataSource) => void;
  onOpenMemoryDetail: (detail: SemanticObjectInspection) => void;
}) {
  return (
    <DataHubView
      source={source}
      onSource={onSource}
      database={<Database onOpenMemory={() => onSource("memory")} />}
      memory={<Memory onOpenDetail={onOpenMemoryDetail} />}
    />
  );
}

export function DataHubView({
  source,
  onSource,
  database,
  memory,
}: {
  source: DataSource;
  onSource: (source: DataSource) => void;
  database: ReactNode;
  memory: ReactNode;
}) {
  const current = sources.find((item) => item.id === source) ?? sources[0];
  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <header className="border-hair flex flex-none items-end gap-8 border-b px-5 pt-4 sm:px-7">
        <div className="pb-3">
          <h1 className="text-ink text-[19px] font-semibold tracking-[-0.02em]">Data</h1>
          <p className="text-fainter mt-0.5 text-[10.5px]">{current.description}</p>
        </div>
        <nav aria-label="Data sources" className="flex self-stretch">
          {sources.map((item) => (
            <button
              key={item.id}
              type="button"
              aria-current={source === item.id ? "page" : undefined}
              onClick={() => onSource(item.id)}
              className={`${source === item.id ? "border-teal text-body" : "border-transparent text-faint hover:text-body"} border-b px-4 text-[11.5px] font-medium`}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </header>
      <div className="flex min-h-0 flex-1 flex-col">{source === "database" ? database : memory}</div>
    </main>
  );
}
