import { useCallback, useEffect, useState } from "react";
import {
  changePlugin,
  inspectSession,
  listPlugins,
  listPresets,
  validatePreset,
  type LifecycleTransition,
  type PluginInspection,
  type PresetInspection,
  type SessionInspection,
} from "../api/management";

export function Management() {
  const [plugins, setPlugins] = useState<PluginInspection | null>(null);
  const [presets, setPresets] = useState<PresetInspection[]>([]);
  const [transition, setTransition] = useState<LifecycleTransition>();
  const [problem, setProblem] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [session, setSession] = useState<SessionInspection>();
  const [validatedPresetId, setValidatedPresetId] = useState<string>();
  const [validatingPresetId, setValidatingPresetId] = useState<string>();

  const refresh = useCallback(async () => {
    try {
      const [pluginResult, presetResult] = await Promise.all([listPlugins(), listPresets()]);
      setPlugins(pluginResult);
      setPresets(presetResult.presets);
      setProblem("");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Management request failed");
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const toggle = async (id: string, enabled: boolean) => {
    try {
      setTransition(await changePlugin(id, enabled));
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Lifecycle request failed");
    }
  };

  const inspect = async () => {
    try {
      setSession(await inspectSession(sessionId));
      setProblem("");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Session inspection failed");
    }
  };

  const validate = async (id: string) => {
    setValidatingPresetId(id);
    try {
      const report = await validatePreset(id);
      setPresets((current) => current.map((preset) => preset.id === report.id ? report : preset));
      setValidatedPresetId(report.id);
      setProblem("");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Preset validation failed");
    } finally {
      setValidatingPresetId(undefined);
    }
  };

  if (!plugins) {
    return <div className="text-muted p-6">{problem || "Loading system diagnostics…"}</div>;
  }
  return (
    <ManagementView
      plugins={plugins}
      presets={presets}
      transition={transition}
      session={session}
      sessionId={sessionId}
      problem={problem}
      validatedPresetId={validatedPresetId}
      validatingPresetId={validatingPresetId}
      onSessionId={setSessionId}
      onInspect={() => void inspect()}
      onToggle={(id, enabled) => void toggle(id, enabled)}
      onValidate={(id) => void validate(id)}
    />
  );
}

export function ManagementView({
  plugins,
  presets,
  transition,
  session,
  sessionId,
  problem,
  validatedPresetId,
  validatingPresetId,
  onSessionId,
  onInspect,
  onToggle,
  onValidate,
}: {
  plugins: PluginInspection;
  presets: PresetInspection[];
  transition?: LifecycleTransition;
  session?: SessionInspection;
  sessionId: string;
  problem?: string;
  validatedPresetId?: string;
  validatingPresetId?: string;
  onSessionId: (value: string) => void;
  onInspect: () => void;
  onToggle: (id: string, enabled: boolean) => void;
  onValidate: (id: string) => void;
}) {
  return (
    <main className="min-h-0 flex-1 overflow-auto p-6">
      <div className="mx-auto flex max-w-[980px] flex-col gap-6">
        <header>
          <h1 className="text-ink text-base font-semibold">System</h1>
          <p className={plugins.degraded ? "text-red mt-1" : "text-muted mt-1"}>
            {plugins.degraded ? "Degraded startup — inspect diagnostics below" : "All required plugin code is healthy"}
          </p>
        </header>
        {problem && <p className="text-red">{problem}</p>}
        {transition && (
          <section className="border-hair bg-surface rounded-lg border p-4">
            <h2 className="text-body font-semibold">Latest lifecycle transition</h2>
            <p className="text-muted mt-2">
              {transition.pluginId}: {transition.from} → {transition.to}
            </p>
            {transition.affectedDependents.length > 0 && (
              <p className="text-amber mt-1">
                Affected dependents: {transition.affectedDependents.map((item) => item.id).join(", ")}
              </p>
            )}
          </section>
        )}
        <section>
          <h2 className="text-body mb-3 font-semibold">Compiled plugins</h2>
          <div className="grid gap-3">
            {plugins.plugins.map((plugin) => (
              <article key={plugin.id} className="border-hair bg-surface rounded-lg border p-4">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <h3 className="text-ink font-mono">{plugin.id}@{plugin.version}</h3>
                    <p className="text-muted mt-1">Lifecycle: {plugin.lifecycle} · Plugin Health: {plugin.health}</p>
                    <p className="text-muted">Connection Readiness: {plugin.connectionReadiness.state}</p>
                  </div>
                  <button className="border-hair rounded border px-3 py-1" onClick={() => onToggle(plugin.id, !plugin.enabled)}>
                    {plugin.enabled ? "Disable" : "Enable"}
                  </button>
                </div>
                <p className="text-fainter mt-2">Required dependencies: {dependencies(plugin.requiredDependencies)}</p>
                <p className="text-fainter">Optional dependencies: {dependencies(plugin.optionalDependencies)}</p>
                {plugin.diagnostic && <p className="text-red mt-2">{plugin.diagnostic}</p>}
                {plugin.connectionReadiness.message && <p className="text-amber mt-2">{plugin.connectionReadiness.message}</p>}
                {plugin.warnings.map((warning) => <p key={warning} className="text-amber mt-1">{warning}</p>)}
              </article>
            ))}
          </div>
        </section>
        <section>
          <h2 className="text-body mb-3 font-semibold">Built-in presets</h2>
          <div className="grid gap-3">
            {presets.map((preset) => (
              <article key={preset.id} className="border-hair bg-surface rounded-lg border p-4">
                <h3 className="text-ink font-mono">{preset.id}@{preset.version}</h3>
                <p className={preset.valid ? "text-green mt-1" : "text-red mt-1"}>
                  {preset.valid ? "Valid" : "Invalid"} · {preset.immutable ? "Immutable built-in" : "Mutable"}
                </p>
                <p className="text-fainter mt-2">Required capabilities: {requirements(preset.requiredCapabilities)}</p>
                <p className="text-fainter">Optional capabilities: {requirements(preset.optionalCapabilities)}</p>
                <button
                  type="button"
                  className="border-hair mt-3 rounded border px-3 py-1"
                  disabled={validatingPresetId === preset.id}
                  onClick={() => onValidate(preset.id)}
                >
                  {validatingPresetId === preset.id ? "Validating…" : "Validate preset"}
                </button>
                {validatedPresetId === preset.id && (
                  <p role="status" className={preset.valid ? "text-green mt-2" : "text-red mt-2"}>
                    Latest validation: {preset.valid ? "valid" : "invalid"}
                  </p>
                )}
                {preset.errors.map((error) => <p key={error} className="text-red mt-1">{error}</p>)}
                {preset.warnings.map((warning) => <p key={warning} className="text-amber mt-1">{warning}</p>)}
              </article>
            ))}
          </div>
        </section>
        <section className="border-hair bg-surface rounded-lg border p-4">
          <h2 className="text-body font-semibold">Session composition inspection</h2>
          <div className="mt-3 flex gap-2">
            <input aria-label="Session ID" className="border-hair bg-app min-w-0 flex-1 rounded border px-3 py-2" value={sessionId} onChange={(event) => onSessionId(event.target.value)} />
            <button className="border-hair rounded border px-3" disabled={!sessionId.trim()} onClick={onInspect}>Inspect</button>
          </div>
          {session && (
            <div className="mt-4 grid gap-3">
              <div><h3 className="text-muted mb-1">Original Composition Receipt</h3><pre className="bg-code overflow-auto rounded p-3 text-xs">{JSON.stringify(session.receipt, null, 2)}</pre></div>
              <div><h3 className="text-muted mb-1">Compatibility Resolutions</h3><pre className="bg-code overflow-auto rounded p-3 text-xs">{JSON.stringify(session.compatibilityResolutions, null, 2)}</pre></div>
            </div>
          )}
        </section>
      </div>
    </main>
  );
}

function requirements(items: { ID: string; Compatibility: { Minimum: string; MaximumExclusive: string } }[]) {
  return items.length ? items.map((item) => `${item.ID} [${item.Compatibility.Minimum},${item.Compatibility.MaximumExclusive})`).join(", ") : "none";
}

const dependencies = requirements;
