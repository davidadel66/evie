import { useState } from "react";
import { parseRunInputs, type WorkflowInterruption } from "../api/workflows";

// A run's durable interruption (Stage 14 sub-stage 7): what the run asks
// David, the data it shows him, the answer's schema and a form for his
// typed answer. A question's answer must fit the step's schema; an effect
// whose outcome is unknown is answered with whether it happened, the
// evidence and, when it happened, its output. Evie checks the answer again
// before the run continues; nothing continues the run without it.

function json(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

/** The answer form starts from an empty answer of the right shape. */
function answerTemplate(interruption: WorkflowInterruption): string {
  return interruption.kind === "effect_reconciliation"
    ? json({outcome: "did-not-happen", evidence: ""})
    : "{}";
}

export function WorkflowInterruptionAnswer({interruption, busy, onAnswer}: {
  interruption: WorkflowInterruption;
  busy: boolean;
  onAnswer: (step: string, answer: Record<string, unknown>) => void;
}) {
  const [text, setText] = useState(() => answerTemplate(interruption));
  const [problem, setProblem] = useState("");
  const effect = interruption.kind === "effect_reconciliation";
  const submit = () => {
    const answer = parseRunInputs(text);
    if (!answer || text.trim() === "") {
      setProblem("The answer must be one JSON object.");
      return;
    }
    setProblem("");
    onAnswer(interruption.step, answer);
  };
  return <section aria-label={`Interruption ${interruption.seq}`} className="border-hair mt-3 rounded-[8px] border p-3">
    <h4 className="text-body font-medium">{effect ? `Did steps.${interruption.step}'s effect happen?` : `steps.${interruption.step} asks`}</h4>
    <p className="text-body mt-1">{interruption.message}</p>
    <p className="text-fainter mt-1">{`Interruption #${interruption.seq}, waiting since ${new Date(interruption.opened_at).toLocaleString()}`}</p>
    <details className="mt-2">
      <summary className="text-muted-text cursor-pointer">{effect ? "Evidence" : "Input"}</summary>
      <pre className="font-mono whitespace-pre-wrap">{json(effect ? interruption.request : interruption.request.input ?? interruption.request)}</pre>
    </details>
    <details className="mt-1">
      <summary className="text-muted-text cursor-pointer">Answer schema</summary>
      <pre className="font-mono whitespace-pre-wrap">{json(interruption.response_schema)}</pre>
    </details>
    <form className="mt-2 flex flex-col gap-2" onSubmit={(event) => { event.preventDefault(); submit(); }}>
      <label className="text-muted-text flex flex-col gap-1">{effect ? "Your reconciliation (a JSON object)" : "Your answer (a JSON object)"}
        <textarea value={text} onChange={(event) => setText(event.target.value)} rows={4} spellCheck={false}
          className="border-hair bg-app text-body rounded-[6px] border px-2 py-1 font-mono" />
      </label>
      {problem && <p role="alert" className="text-danger">{problem}</p>}
      <div><button type="submit" disabled={busy} className="bg-teal text-primary-foreground rounded-[6px] px-3 py-1 font-semibold disabled:opacity-40">Answer</button></div>
    </form>
  </section>;
}
