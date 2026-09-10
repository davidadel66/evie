// The one file that knows which markdown renderer we use. Streamdown handles
// the streaming edge cases (unterminated fences, half-written links) and
// memoizes per block, so a delta only re-renders the block it touched.

import { code, evieCodeTheme } from "./codeHighlight";
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
      className="evie-markdown space-y-[12px]"
      plugins={{ code }}
      shikiTheme={[evieCodeTheme, evieCodeTheme]}
      controls={{
        code: { copy: true, download: false },
        table: false,
        mermaid: false,
      }}
    >
      {text}
    </Streamdown>
  );
}
