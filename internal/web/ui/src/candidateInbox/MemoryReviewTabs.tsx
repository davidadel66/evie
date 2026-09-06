import { useState, type ReactNode } from "react";
import { CandidateInbox } from "./CandidateInbox";
import { CompilerHealth } from "../compilerDiagnostics/CompilerHealth";

export function MemoryReviewTabs({ children }: { children: ReactNode }) {
  const [view, setView] = useState<"accepted" | "review" | "health">("accepted");
  return <section className="flex min-h-0 flex-1 flex-col overflow-hidden" aria-label="Memory workspace">
    <nav className="border-hair flex flex-none items-center gap-6 border-b px-5 sm:px-7" aria-label="Memory views">
      <button type="button" className={`${view === "accepted" ? "border-teal text-ink" : "border-transparent text-muted-text hover:text-body"} border-b-2 py-3 text-sm`} aria-pressed={view === "accepted"} onClick={() => setView("accepted")}>Memories</button>
      <button type="button" className={`${view === "review" ? "border-teal text-ink" : "border-transparent text-muted-text hover:text-body"} border-b-2 py-3 text-sm`} aria-pressed={view === "review"} onClick={() => setView("review")}>Review</button>
      <div className="flex-1" />
      <details className="relative py-2"><summary aria-label="Memory tools" className="text-muted-text hover:text-body cursor-pointer rounded px-2 py-1 text-sm">More</summary><div className="border-hair bg-sidebar absolute right-0 top-full z-40 w-52 rounded-lg border p-1 shadow-lg"><button type="button" className="text-body hover:bg-hover w-full rounded px-3 py-2 text-left text-sm" onClick={(event) => { setView("health"); event.currentTarget.closest("details")?.removeAttribute("open"); }}>Background activity</button></div></details>
    </nav>
    {view === "review" ? <CandidateInbox /> : view === "health" ? <CompilerHealth /> : children}
  </section>;
}
