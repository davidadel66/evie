import { useState, type ReactNode } from "react";
import { CandidateInbox } from "./CandidateInbox";
import { CompilerHealth } from "../compilerDiagnostics/CompilerHealth";

export function MemoryReviewTabs({ children }: { children: ReactNode }) {
  const [view, setView] = useState<"accepted" | "review" | "health">("accepted");
  return <section className="flex min-h-0 flex-1 flex-col overflow-hidden" aria-label="Memory workspace">
    <nav className="border-hair flex flex-none gap-4 border-b px-5 py-3" aria-label="Memory views">
      <button type="button" className={view === "accepted" ? "text-teal" : "text-faint"} aria-pressed={view === "accepted"} onClick={() => setView("accepted")}>Accepted memory</button>
      <button type="button" className={view === "review" ? "text-teal" : "text-faint"} aria-pressed={view === "review"} onClick={() => setView("review")}>Review candidates</button>
      <button type="button" className={view === "health" ? "text-teal" : "text-faint"} aria-pressed={view === "health"} onClick={() => setView("health")}>Compiler health</button>
    </nav>
    {view === "review" ? <CandidateInbox /> : view === "health" ? <CompilerHealth /> : children}
  </section>;
}
