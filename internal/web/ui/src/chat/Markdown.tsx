// The one file that knows which markdown renderer we use. Streamdown handles
// the streaming edge cases (unterminated fences, half-written links) and
// memoizes per block, so a delta only re-renders the block it touched.

import { Streamdown } from "streamdown";

type Props = {
  text: string;
  /** While true, Streamdown patches incomplete syntax instead of showing raw
   *  markers — the difference between reading prose and watching asterisks. */
  streaming: boolean;
};

export function Markdown({ text, streaming }: Props) {
  return (
    <Streamdown
      mode={streaming ? "streaming" : "static"}
      parseIncompleteMarkdown={streaming}
      // Design: body text #c9cfcb, 1.6 line height, bold lifts to #e2e6e3.
      className="text-body space-y-[10px] text-[13px] leading-[1.6] [&_b]:text-ink [&_strong]:text-ink"
      controls={{ code: true, table: false, mermaid: false }}
    >
      {text}
    </Streamdown>
  );
}
