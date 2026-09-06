import type { ReactNode } from "react";
import type { ContextSessionSnapshot } from "../api/contextSessions";
import { Memory } from "../memory/Memory";
import { Database } from "./Database";
import { Usage } from "./Usage";

export type DataSource = "database" | "memory" | "usage";

const sources: { id: DataSource; label: string; description: string }[] = [
  { id: "database", label: "Database", description: "Physical schema and approved records" },
  { id: "memory", label: "Memory", description: "Scoped accepted knowledge" },
  { id: "usage", label: "Usage", description: "Token usage and collection coverage" },
];

export function DataHub({
  source,
  onSource,
  snapshot,
}: {
  source: DataSource;
  onSource: (source: DataSource) => void;
  snapshot?: ContextSessionSnapshot;
}) {
  return (
    <DataHubView
      source={source}
      onSource={onSource}
      database={<Database onOpenMemory={() => onSource("memory")} />}
      memory={<Memory snapshot={snapshot} />}
      usage={<Usage />}
    />
  );
}

export function DataHubView({
  source,
  onSource,
  database,
  memory,
  usage,
}: {
  source: DataSource;
  onSource: (source: DataSource) => void;
  database: ReactNode;
  memory: ReactNode;
  usage?: ReactNode;
}) {
  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <header className="border-hair flex flex-none items-end gap-8 border-b px-5 pt-4 sm:px-7">
        <div className="pb-3">
          <h1 className="text-ink text-[19px] font-semibold tracking-[-0.02em]">Data</h1>
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
      <div className="flex min-h-0 flex-1 flex-col">{source === "database" ? database : source === "memory" ? memory : usage}</div>
    </main>
  );
}
