// The one file that knows which markdown renderer we use. Streamdown handles
// the streaming edge cases (unterminated fences, half-written links) and
// memoizes per block, so a delta only re-renders the block it touched.

import { createCodePlugin, type ThemeInput } from "@streamdown/code";
import { Streamdown } from "streamdown";

// A deliberately small syntax palette: teal identifies names/functions,
// amber marks language structure, green is data, and gray recedes comments.
// Keeping punctuation and variables near the body color avoids rainbow code.
const evieCodeTheme: ThemeInput = {
  name: "evie-dark",
  type: "dark",
  colors: {
    "editor.background": "#0b0e10",
    "editor.foreground": "#c9cfcb",
  },
  tokenColors: [
    {
      scope: ["comment", "punctuation.definition.comment"],
      settings: { foreground: "#8b9491", fontStyle: "italic" },
    },
    {
      scope: ["keyword", "storage.type", "storage.modifier"],
      settings: { foreground: "#d9a04a" },
    },
    {
      scope: ["string", "string.quoted", "string.regexp"],
      settings: { foreground: "#a8d4b5" },
    },
    {
      scope: ["constant.numeric", "constant.language", "constant.character"],
      settings: { foreground: "#e8c98a" },
    },
    {
      scope: [
        "entity.name.function",
        "entity.name.type",
        "entity.name.class",
        "support.function",
        "support.type",
        "support.class",
      ],
      settings: { foreground: "#6fd0be" },
    },
    {
      scope: ["variable", "variable.parameter", "punctuation", "keyword.operator"],
      settings: { foreground: "#c9cfcb" },
    },
  ],
};

const code = createCodePlugin({ themes: [evieCodeTheme, evieCodeTheme] });

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
