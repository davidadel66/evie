// The one file that knows which markdown renderer we use. Streamdown handles
// the streaming edge cases (unterminated fences, half-written links) and
// memoizes per block, so a delta only re-renders the block it touched.

import type { ReactNode } from "react";
import { code, evieCodeTheme } from "./codeHighlight";
import { Streamdown, defaultRehypePlugins, type Components } from "streamdown";

type Props = {
  text: string;
  /** While true, Streamdown patches incomplete syntax instead of showing raw
   *  markers — the difference between reading prose and watching asterisks. */
  streaming: boolean;
  /** For text Evie did not write, such as a worker's report quoting web
   *  pages (artifacts.spec.md): raw HTML shows as text, only http(s) links
   *  are links and open outside Evie with no opener or referrer, and images
   *  are not loaded. Chat messages leave it off. */
  inert?: boolean;
};

export function Markdown({ text, streaming, inert = false }: Props) {
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
      {...(inert ? inertOptions : {})}
    >
      {text}
    </Streamdown>
  );
}

/** The URL when it is an absolute http(s) URL, else undefined. */
function webURL(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:" ? url.href : undefined;
  } catch {
    return undefined;
  }
}

function InertAnchor({ href, children }: { href: string; children: ReactNode }) {
  return <a href={href} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer" data-streamdown="link" className="underline">{children}</a>;
}

// Streamdown's own link is a button behind a confirmation without an href;
// an inert link is a plain anchor, or plain text when it is not http(s).
const inertComponents: Components = {
  a: ({ href, children }) => {
    const url = webURL(href);
    return url ? <InertAnchor href={url}>{children}</InertAnchor> : <span>{children}</span>;
  },
  // Never an <img>: React would also preload it. Its alt text stands in,
  // linked to the image when that is an http(s) URL.
  img: ({ src, alt }) => {
    const url = webURL(src);
    const label = alt || url || "image";
    return url ? <InertAnchor href={url}>{label}</InertAnchor> : <span>{label}</span>;
  },
};

// Without the raw plugin, HTML in the text is never parsed and stays text.
const inertOptions = {
  rehypePlugins: [defaultRehypePlugins.sanitize, defaultRehypePlugins.harden],
  components: inertComponents,
};
