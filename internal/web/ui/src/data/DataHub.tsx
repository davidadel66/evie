import type { ReactNode } from "react";
import type { ContextSessionSnapshot } from "../api/contextSessions";
import { Memory } from "../memory/Memory";
import { Database } from "./Database";
import { Usage } from "./Usage";
import { Spending } from "./Spending";

export type DataSource = "database" | "memory" | "usage" | "spending";

const sources: { id: DataSource; label: string; description: string }[] = [
  { id: "database", label: "Database", description: "Physical schema and approved records" },
  { id: "memory", label: "Memory", description: "Scoped accepted knowledge" },
  { id: "usage", label: "Usage", description: "Token usage and collection coverage" },
  { id: "spending", label: "Spending", description: "Bank transactions and daily net flow" },
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
      spending={<Spending />}
    />
  );
}

export function DataHubView({
  source,
  onSource,
  database,
  memory,
  usage,
  spending,
}: {
  source: DataSource;
  onSource: (source: DataSource) => void;
  database: ReactNode;
  memory: ReactNode;
  usage?: ReactNode;
  spending?: ReactNode;
}) {
  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <header className="border-hair flex min-w-0 flex-none items-end gap-3 border-b px-5 pt-4 sm:gap-8 sm:px-7">
        <div className="pb-3">
          <h1 className="text-ink text-[19px] font-semibold tracking-[-0.02em]">Data</h1>
        </div>
        <nav aria-label="Data sources" className="flex min-w-0 self-stretch overflow-x-auto">
          {sources.map((item) => (
            <button
              key={item.id}
              type="button"
              aria-current={source === item.id ? "page" : undefined}
              onClick={() => onSource(item.id)}
              className={`${source === item.id ? "border-teal text-body" : "border-transparent text-faint hover:text-body"} focus-visible:ring-teal flex-none border-b px-3 text-[11.5px] font-medium focus-visible:ring-2 focus-visible:outline-none sm:px-4`}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </header>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">{source === "database" ? database : source === "memory" ? memory : source === "usage" ? usage : spending}</div>
    </main>
  );
}
