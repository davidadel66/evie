import { useEffect, useState } from "react";
import type { HighlightResult } from "@streamdown/code";
import { code, evieCodeTheme, exactHighlightTokens } from "../chat/codeHighlight";
import { Diff } from "../chat/Diff";
import type { FileInspection } from "./fileInspection";

export function FileViewer({ file }: { file: FileInspection }) {
  const [view, setView] = useState<"file" | "changes" | "details">(file.content.kind === "change" ? "changes" : "file");
  const content = file.content;
  const change = content.kind === "change" ? content : undefined;
  const proposed = !!change && file.status !== "Edited";
  const text = content.kind === "code" ? content.text : change?.after ?? "";
  const partial = content.kind === "code" ? content.coverage === "excerpt" : change?.coverage === "replacement";
  const description = content.kind === "code"
    ? partial ? "Recorded excerpt; the complete file is not available in this result." : "Recorded file contents"
    : change
      ? partial ? "Recorded replacement only; the rest of the file was not saved with this edit." : proposed ? "Proposed file contents" : "Recorded contents after this edit" : "File preview unavailable";
  return <div className="flex min-h-0 flex-1 flex-col">
    <div className="border-hair flex-none border-b px-4 py-3">
      <p className="text-body break-all font-mono text-xs">{file.path}</p>
      <p className={`mt-2 text-xs ${file.status === "Failed" ? "text-danger-ink" : proposed ? "text-amber-ink" : "text-muted-text"}`}>{file.status}</p>
      {file.approvalState && <p className="text-muted-text mt-1 text-xs">Approval: {file.approvalState[0].toUpperCase() + file.approvalState.slice(1)}</p>}
      <p className="text-muted-text mt-1 text-xs leading-5">{description}</p>
    </div>
    {(change || file.tool) && <div role="tablist" aria-label="File view" className="border-hair flex flex-none gap-5 border-b px-4">
      {(["file", ...(change ? ["changes" as const] : []), ...(file.tool ? ["details" as const] : [])] as const).map((tab, index, tabs) => <button key={tab} type="button" role="tab" aria-selected={view === tab} onClick={() => setView(tab)} onKeyDown={(event) => {
        if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
          event.preventDefault();
          const next = (index + (event.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length;
          setView(tabs[next]);
          (event.currentTarget.parentElement?.children[next] as HTMLButtonElement | undefined)?.focus();
        }
      }} className={`focus-visible:outline-teal cursor-pointer border-b-2 py-2.5 text-xs focus-visible:outline-2 ${view === tab ? "border-teal text-ink" : "text-muted-text border-transparent hover:text-body"}`}>
        {tab === "details" ? "Details" : tab === "changes" ? "Changes" : partial ? "Replacement" : proposed ? "Proposed file" : "File"}
      </button>)}
    </div>}
    <div className="bg-code min-h-0 flex-1 overflow-auto" role={change || file.tool ? "tabpanel" : undefined} aria-label={view === "details" ? "Tool details" : change ? view === "changes" ? "Changes" : "File contents" : "Recorded file contents"}>
      {view === "details" && file.tool ? <div className="text-body space-y-3 p-4 text-xs"><p>{file.tool.name}</p><p className="text-muted-text">Arguments</p><pre className="overflow-auto whitespace-pre-wrap break-all">{file.tool.args}</pre><p className="text-muted-text">Result</p><pre className="overflow-auto whitespace-pre-wrap break-all">{file.tool.result ?? "No result recorded yet."}</pre></div>
        : content.kind === "unavailable" ? <p className="text-muted-text p-4 text-sm whitespace-pre-wrap">{content.message}</p>
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
    const result = code.highlight({code: text, language, themes: [evieCodeTheme, evieCodeTheme]}, accept);
    if (result) accept(result);
    return () => { active = false; };
  }, [text, path, language]);
  if (text === "") return <p className="text-muted-text p-4 text-xs">Empty file</p>;
  const lines = text.split("\n");
  if (text.endsWith("\n")) lines.pop();
  const tokens = exactHighlightTokens(highlighted?.text === text && highlighted.path === path ? highlighted.result : undefined, text);
  return <div className="min-w-max py-3 font-mono text-[12px] leading-[1.75]">
    <div className="flex">
      {numbered && <pre aria-hidden="true" className="text-faint sticky left-0 bg-code m-0 min-w-12 flex-none px-3 text-right select-none">{lines.map((_, i) => i + 1).join("\n")}</pre>}
      <pre aria-label="Source code" className="text-body m-0 flex-1 px-4 whitespace-pre">{tokens ? tokens.slice(0, lines.length).map((line, i) => <span key={i}>{line.map((token, j) => <span key={j} style={token.htmlStyle ?? {color: token.color}}>{token.content}</span>)}{i < lines.length - 1 ? "\n" : ""}</span>) : text}</pre>
    </div>
    {!text.endsWith("\n") && numbered && <p className="text-muted-text mt-3 px-4 text-[10px]">No newline at end of file</p>}
  </div>;
}
