import { useState, type ReactNode } from "react";
import { CandidateInbox } from "./CandidateInbox";

export function MemoryReviewTabs({ children }: { children: ReactNode }) {
  const [review, setReview] = useState(false);
  return <section className="flex min-h-0 flex-1 flex-col overflow-hidden" aria-label="Memory workspace">
    <nav className="border-hair flex flex-none gap-4 border-b px-5 py-3" aria-label="Memory views">
      <button type="button" className={review ? "text-faint" : "text-teal"} aria-pressed={!review} onClick={() => setReview(false)}>Accepted memory</button>
      <button type="button" className={review ? "text-teal" : "text-faint"} aria-pressed={review} onClick={() => setReview(true)}>Review candidates</button>
    </nav>
    {review ? <CandidateInbox /> : children}
  </section>;
}
