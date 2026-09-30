import { useEffect, useId, useRef, useState } from "react";
import { chooseWorkspaceFolder, type WorkspaceCreation } from "../api/contextSessions";
import { Cross, Folder } from "../ui/Icon";
import { listPresets, type PresetInspection } from "../api/management";
import { Dialog } from "../ui/Dialog";
import { presetDescription, presetLabel, workspaceCreation, type WorkspaceDraft } from "./workspaceCreation";

const input = "border-hair-input bg-card text-ink placeholder:text-faint focus-visible:ring-teal w-full min-w-0 rounded-lg border px-3 py-2.5 text-[13px] focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50";
const initialDraft: WorkspaceDraft = { name: "", presetId: "standard", folderPath: "" };

export function CreateWorkspaceDialog({ onClose, onCreate }: { onClose: () => void; onCreate: (options: WorkspaceCreation) => Promise<void> }) {
  const [draft, setDraft] = useState(initialDraft);
  const [presets, setPresets] = useState<PresetInspection[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadProblem, setLoadProblem] = useState("");
  const [problem, setProblem] = useState("");
  const [retry, setRetry] = useState(0);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [pickingFolder, setPickingFolder] = useState(false);
  const [folderProblem, setFolderProblem] = useState("");
  const pickerRequest = useRef<AbortController | null>(null);
  const pickerButton = useRef<HTMLButtonElement>(null);
  const nameId = useId();
  const presetId = useId();
  const presetHelpId = useId();
  const folderHelpId = useId();

  useEffect(() => {
    let current = true;
    setLoading(true);
    setLoadProblem("");
    listPresets().then(({ presets: available }) => {
      if (current) setPresets(available);
    }).catch((error: unknown) => {
      if (current) setLoadProblem(error instanceof Error ? error.message : "Agent presets could not be loaded.");
    }).finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [retry]);

  useEffect(() => () => { pickerRequest.current?.abort(); }, []);

  const chooseFolder = async () => {
    if (pickerRequest.current || savingRef.current) return;
    const request = new AbortController();
    pickerRequest.current = request;
    setPickingFolder(true);
    setFolderProblem("");
    try {
      const choice = await chooseWorkspaceFolder(request.signal);
      if (!request.signal.aborted && !choice.cancelled) {
        setDraft(current => ({ ...current, folderPath: choice.path }));
        setProblem("");
      }
    } catch (error: unknown) {
      if (!request.signal.aborted) setFolderProblem(error instanceof Error ? error.message : "The folder picker could not be opened. Try again.");
    } finally {
      if (!request.signal.aborted) {
        pickerRequest.current = null;
        setPickingFolder(false);
        requestAnimationFrame(() => pickerButton.current?.focus());
      }
    }
  };

  const selectedPreset = presets.find((preset) => preset.id === draft.presetId);
  const update = (patch: Partial<WorkspaceDraft>) => { setDraft((value) => ({ ...value, ...patch })); setProblem(""); };
  const submit = async () => {
    if (savingRef.current || pickerRequest.current || loading || loadProblem) return;
    let options: WorkspaceCreation;
    try {
      options = workspaceCreation(draft, presets);
    } catch (error: unknown) {
      setProblem(error instanceof Error ? error.message : "Check the workspace details.");
      return;
    }
    savingRef.current = true;
    setSaving(true);
    setProblem("");
    try {
      await onCreate(options);
    } catch (error: unknown) {
      setProblem(error instanceof Error ? error.message : "Workspace could not be created. Try again.");
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  return (
    <Dialog title="Create workspace" description="Give your conversations a place of their own." busy={saving || pickingFolder} onClose={onClose}>
      <form onSubmit={(event) => { event.preventDefault(); void submit(); }} className="space-y-5">
        <div>
          <label htmlFor={nameId} className="text-body mb-2 block text-xs font-medium">Workspace name</label>
          <input id={nameId} data-dialog-autofocus required autoComplete="off" value={draft.name} onChange={(event) => update({ name: event.target.value })} disabled={saving} placeholder="e.g. Interview prep" className={input} />
        </div>

        <div>
          <label htmlFor={presetId} className="text-body mb-2 block text-xs font-medium">Agent preset</label>
          <select id={presetId} aria-describedby={presetHelpId} value={draft.presetId} disabled={loading || Boolean(loadProblem) || saving} onChange={(event) => update({ presetId: event.target.value, allowResearchDelegation: false })} className={input}>
            {!presets.some((preset) => preset.id === "standard") && <option value="standard">Standard{loading ? " — loading…" : " — unavailable"}</option>}
            {presets.map((preset) => <option key={preset.id} value={preset.id} disabled={!preset.valid}>{presetLabel(preset.id)}{preset.valid ? "" : " — unavailable"}</option>)}
          </select>
          <p id={presetHelpId} className="text-muted-text mt-2 text-xs leading-5">
            {selectedPreset ? presetDescription(selectedPreset) : "Choose which plugin capabilities this workspace can use."}
          </p>
          {loadProblem && <div className="mt-2 text-xs leading-5"><p role="alert" className="text-danger-ink max-h-24 overflow-y-auto break-words">{loadProblem}</p><button type="button" onClick={() => setRetry((value) => value + 1)} className="text-body focus-visible:ring-teal mt-1 rounded underline underline-offset-2 focus-visible:ring-2 focus-visible:outline-none">Retry</button></div>}
          {!loading && !loadProblem && <PresetDiagnostics preset={selectedPreset} />}
          {draft.presetId === "standard" && <div className="mt-4">
            <label className="text-body flex items-center gap-2 text-xs"><input type="checkbox" className="accent-teal" checked={draft.allowResearchDelegation ?? false} disabled={saving || loading || !presets.some(preset => preset.id === "research" && preset.valid)} onChange={event => update({allowResearchDelegation: event.target.checked})} />Allow research delegation</label>
            <p className="text-muted-text mt-1 pl-5 text-xs leading-5">Let eligible chats send web research to separate workers. Workers receive only the assignment and selected context.</p>
          </div>}
        </div>

        <fieldset disabled={saving || pickingFolder}>
          <legend className="text-body mb-2 text-xs font-medium">Local folder <span className="text-muted-text font-normal">(optional)</span></legend>
          <button ref={pickerButton} type="button" onClick={() => void chooseFolder()} aria-describedby={folderHelpId} className="border-hair-input text-body hover:bg-hover hover:text-ink focus-visible:ring-teal flex items-center gap-2 rounded-lg border px-3 py-2.5 text-xs font-medium focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50">
            <Folder size={14} /> {pickingFolder ? "Choosing folder…" : "Choose folder…"}
          </button>
          {draft.folderPath ? <div className="border-hair bg-card mt-3 flex items-start gap-3 rounded-lg border px-3 py-2.5">
            <p className="text-body min-w-0 flex-1 break-words text-xs leading-5" aria-label="Selected folder">{draft.folderPath}</p>
            <button type="button" aria-label="Remove selected folder" title="Remove folder" onClick={() => { update({ folderPath: "" }); setFolderProblem(""); pickerButton.current?.focus(); }} className="text-muted-text hover:text-ink focus-visible:ring-teal -mr-1 rounded p-1 focus-visible:ring-2 focus-visible:outline-none"><Cross size={13} /></button>
          </div> : null}
          <p id={folderHelpId} className="text-muted-text mt-2 text-xs leading-5">Choose a folder, or use New Folder in the macOS picker.</p>
          {pickingFolder && <p role="status" className="text-muted-text mt-2 text-xs">Choose a folder in the macOS window, or cancel to return here.</p>}
          {folderProblem && <p role="alert" className="text-danger-ink mt-2 text-xs leading-5">{folderProblem}</p>}
        </fieldset>

        {problem && <p role="alert" className="text-danger-ink max-h-24 overflow-y-auto break-words text-xs leading-5">{problem}</p>}
        <footer className="border-hair flex items-center justify-end gap-2 border-t pt-5">
          <button type="button" disabled={saving || pickingFolder} onClick={onClose} className="text-muted-text hover:bg-hover hover:text-ink focus-visible:ring-teal rounded-lg px-4 py-2.5 text-xs font-medium focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">Cancel</button>
          <button type="submit" disabled={saving || pickingFolder || loading || Boolean(loadProblem) || !draft.name.trim() || !selectedPreset?.valid} className="bg-teal text-primary-foreground hover:bg-teal-hover focus-visible:ring-teal rounded-lg px-4 py-2.5 text-xs font-semibold focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">{saving ? "Creating…" : "Create workspace"}</button>
        </footer>
      </form>
    </Dialog>
  );
}

export function PresetDiagnostics({ preset }: { preset?: PresetInspection }) {
  const messages = preset?.valid ? preset.warnings ?? [] : preset?.errors ?? [];
  return <>
    {!preset?.valid && <p role="alert" className="text-danger-ink mt-2 text-xs leading-5">This preset is unavailable. Choose another preset or check plugin settings.</p>}
    {messages.length > 0 && <details className={`${preset?.valid ? "text-amber-ink" : "text-danger-ink"} mt-2 text-xs leading-5`}>
      <summary className="focus-visible:ring-teal cursor-pointer rounded focus-visible:ring-2 focus-visible:outline-none">{preset?.valid ? "Some optional capabilities are unavailable" : "Show unavailable capabilities"}</summary>
      <ul aria-label="Capability details" className="mt-2 max-h-32 list-disc space-y-2 overflow-y-auto pl-5 pr-2 break-words">
        {messages.map((message, index) => <li key={index}>{message}</li>)}
      </ul>
    </details>}
  </>;
}
