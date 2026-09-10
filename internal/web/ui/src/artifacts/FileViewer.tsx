import { useEffect, useState } from "react";
import type { HighlightResult } from "@streamdown/code";
import { code, fileCodeTheme, exactHighlightTokens } from "../chat/codeHighlight";
import { Diff } from "../chat/Diff";
import type { FileInspection } from "./fileInspection";

export function FilePath({ file }: { file: FileInspection }) {
  const segments = file.path.split("/").filter(Boolean);
  const approval = file.approvalState ? `\nApproval: ${file.approvalState[0].toUpperCase() + file.approvalState.slice(1)}` : "";
  return <div aria-label={`File path: ${file.path}`} title={file.path + approval} className="flex min-w-0 flex-1 items-center whitespace-nowrap text-[13px]">
    {segments.length > 1 && <span className="text-editor-muted min-w-0 truncate">{segments.slice(0, -1).join("  ›  ")}  ›</span>}
    <span className="text-editor-text max-w-full flex-none truncate pl-2 font-medium">{segments.at(-1) ?? file.path}</span>
  </div>;
}

export function FileViewer({ file }: { file: FileInspection }) {
  const [view, setView] = useState<"file" | "changes">(file.content.kind === "change" ? "changes" : "file");
  const content = file.content;
  const change = content.kind === "change" ? content : undefined;
  const text = content.kind === "code" ? content.text : change?.after ?? "";
  const partial = content.kind === "code" ? content.coverage === "excerpt" : change?.coverage === "replacement";
  const qualifier = [
    change && file.status !== "Edited" ? `${file.status} — proposed change` : "",
    partial ? content.kind === "code" ? "Partial file" : "Replacement only" : "",
  ].filter(Boolean).join(" · ");
  return <div className="flex min-h-0 flex-1 flex-col">
    {(change || qualifier) && <div className="border-editor-hair flex flex-none items-center gap-4 border-b px-4">
      {change && <div role="tablist" aria-label="File view" className="flex flex-none gap-4">
        {(["file", "changes"] as const).map((tab) => <button key={tab} type="button" role="tab" aria-selected={view === tab} tabIndex={view === tab ? 0 : -1} onClick={() => setView(tab)} onKeyDown={(event) => {
          if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
            event.preventDefault();
            const next = tab === "file" ? "changes" : "file";
            setView(next);
            (event.currentTarget.parentElement?.children[next === "file" ? 0 : 1] as HTMLButtonElement | undefined)?.focus();
          }
        }} className={`focus-visible:outline-teal cursor-pointer border-b-2 py-2 text-xs focus-visible:outline-2 ${view === tab ? "border-teal text-editor-text" : "text-editor-muted border-transparent hover:text-editor-text"}`}>
          {tab === "changes" ? "Changes" : partial ? "Replacement" : "File"}
        </button>)}
      </div>}
      {qualifier && <span className="text-editor-muted py-2 text-[11px]" title={partial ? "Only the recorded excerpt or replacement is available; this is not the complete file." : undefined}>{qualifier}</span>}
    </div>}
    <div className="bg-editor focus-visible:outline-teal min-h-0 flex-1 overflow-auto focus-visible:outline-1 focus-visible:-outline-offset-1" tabIndex={0} role={change ? "tabpanel" : "region"} aria-label={change && view === "changes" ? "Changes" : "File contents"}>
      {content.kind === "unavailable" ? <p className="text-editor-muted p-4 text-sm whitespace-pre-wrap">{content.message}</p>
        : view === "changes" && change ? <Diff oldText={change.before} newText={change.after} isNew={change.isNew} fit partial={partial} />
          : <FileCode text={text} path={file.path} numbered={!partial} />}
    </div>
  </div>;
}

const extensions: Record<string, string> = {go:"go",ts:"typescript",tsx:"tsx",js:"javascript",jsx:"jsx",mjs:"javascript",cjs:"javascript",json:"json",py:"python",rs:"rust",md:"markdown",css:"css",html:"html",sh:"shellscript",bash:"shellscript",zsh:"shellscript",yaml:"yaml",yml:"yaml",toml:"toml",sql:"sql",txt:"text"};

function fileLanguage(path: string) {
  const name = path.split("/").pop()?.toLowerCase() ?? "";
  const candidate = name === "dockerfile" ? "dockerfile" : extensions[name.split(".").pop() ?? ""];
  return code.getSupportedLanguages().find((language) => language === candidate);
}

export function FileCode({text, path, numbered = true}: {text: string; path: string; numbered?: boolean}) {
  const language = fileLanguage(path);
  const [highlighted, setHighlighted] = useState<{text: string; path: string; result: HighlightResult} | null>(null);
  useEffect(() => {
    let active = true;
    if (!language || text.length > 100 * 1024) return;
    const accept = (result: HighlightResult) => { if (active) setHighlighted({text, path, result}); };
    const result = code.highlight({code: text, language, themes: [fileCodeTheme, fileCodeTheme]}, accept);
    if (result) accept(result);
    return () => { active = false; };
  }, [text, path, language]);
  if (text === "") return <p className="text-muted-text p-4 text-xs">Empty file</p>;
  const lines = text.split("\n");
  if (text.endsWith("\n")) lines.pop();
  const tokens = exactHighlightTokens(highlighted?.text === text && highlighted.path === path ? highlighted.result : undefined, text);
  return <div className="min-w-max py-3 font-mono text-[13px] leading-[1.8]">
    <div className="flex">
      {numbered && <pre aria-hidden="true" className="text-editor-muted sticky left-0 bg-editor m-0 min-w-12 flex-none px-3 text-right select-none">{lines.map((_, i) => i + 1).join("\n")}</pre>}
      <pre aria-label="Source code" className="text-editor-text m-0 flex-1 pr-6 pl-3 whitespace-pre">{tokens ? tokens.slice(0, lines.length).map((line, i) => <span key={i}>{line.map((token, j) => <span key={j} style={token.htmlStyle ?? {color: token.color}}>{token.content}</span>)}{i < lines.length - 1 ? "\n" : ""}</span>) : text}</pre>
    </div>
    {!text.endsWith("\n") && numbered && <p className="text-muted-text mt-3 px-4 text-[10px]">No newline at end of file</p>}
  </div>;
}
