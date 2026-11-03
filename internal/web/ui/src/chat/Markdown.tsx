// The one file that knows which markdown renderer we use. Streamdown handles
// the streaming edge cases (unterminated fences, half-written links) and
// memoizes per block, so a delta only re-renders the block it touched.

import { createContext, useContext, type ReactNode } from "react";
import { code, evieCodeTheme } from "./codeHighlight";
import { Streamdown, defaultRehypePlugins, type Components, type ControlsConfig, type PluginConfig, type ThemeInput } from "streamdown";

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
      plugins={plugins}
      shikiTheme={shikiTheme}
      controls={controls}
      {...(inert ? inertOptions : {})}
    >
      {text}
    </Streamdown>
  );
}

// Streamdown memoizes on its props, so these stay the same objects from one
// render to the next: a parent that re-renders, such as a ticking clock,
// does not make it parse a long document again.
const plugins: PluginConfig = { code };
const shikiTheme: [ThemeInput, ThemeInput] = [evieCodeTheme, evieCodeTheme];
const controls: ControlsConfig = {
  code: { copy: true, download: false },
  table: false,
  mermaid: false,
};

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

// Set inside an inert link, whose content must not be a link of its own.
const InsideInertLink = createContext(false);

/** A URL from text Evie did not write, such as a report's link or one of
 *  its sources: a link that opens outside Evie with no opener or referrer
 *  when it is http(s), else plain text in the same style. */
export function InertLink({ href, className, "data-streamdown": streamdown, children }: {
  href: unknown; className?: string; "data-streamdown"?: string; children: ReactNode;
}) {
  const url = webURL(href);
  if (!url) return <span className={className}>{children}</span>;
  return <a href={url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer" data-streamdown={streamdown} className={className}>
    <InsideInertLink.Provider value>{children}</InsideInertLink.Provider>
  </a>;
}

// Never an <img>: React would also preload it. Its alt text stands in,
// linked to the image when that is an http(s) URL, or as plain text inside
// a link, since a link cannot hold another.
function InertImage({ src, alt }: { src: unknown; alt?: string }) {
  const inLink = useContext(InsideInertLink);
  const label = alt || webURL(src) || "image";
  return inLink ? <span>{label}</span> : <InertLink href={src} data-streamdown="link" className="underline">{label}</InertLink>;
}

// Streamdown's own link is a button behind a confirmation without an href;
// an inert link is a plain anchor, or plain text when it is not http(s).
const inertComponents: Components = {
  a: ({ href, children }) => <InertLink href={href} data-streamdown="link" className="underline">{children}</InertLink>,
  img: ({ src, alt }) => <InertImage src={src} alt={alt} />,
};

// Without the raw plugin, HTML in the text is never parsed and stays text.
const inertOptions = {
  rehypePlugins: [defaultRehypePlugins.sanitize, defaultRehypePlugins.harden],
  components: inertComponents,
};
