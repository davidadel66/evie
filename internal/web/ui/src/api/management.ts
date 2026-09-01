export type VersionRange = { Minimum: string; MaximumExclusive: string };
export type Dependency = { ID: string; Compatibility: VersionRange };
export type LifecycleState =
  | "disabled"
  | "waiting"
  | "loading"
  | "ready"
  | "failed"
  | "stopping"
  | "stopped";
export type PluginHealth = "healthy" | "degraded" | "unhealthy";
export type PluginDiagnosticCode =
  | "plugin_failed"
  | "cleanup_pending"
  | "dependency_unavailable";
export type ConnectionReadiness = {
  state: "not_required" | "ready" | "not_ready";
  code?: "connection_not_ready";
  message?: string;
};
export type PluginStatus = {
  id: string;
  version: string;
  enabled: boolean;
  lifecycle: LifecycleState;
  requiredDependencies: Dependency[];
  optionalDependencies: Dependency[];
  health: PluginHealth;
  connectionReadiness: ConnectionReadiness;
  diagnosticCode?: PluginDiagnosticCode;
  diagnostic?: string;
  warnings: string[];
};
export type PluginInspection = { degraded: boolean; plugins: PluginStatus[] };
export type CapabilityRequirement = { ID: string; Compatibility: VersionRange };
export type PresetInspection = {
  id: string;
  version: string;
  requiredCapabilities: CapabilityRequirement[];
  optionalCapabilities: CapabilityRequirement[];
  valid: boolean;
  warnings: string[];
  errors: string[];
  immutable: boolean;
};
export type LifecycleTransition = {
  pluginId: string;
  from: LifecycleState;
  to: LifecycleState;
  enabled: boolean;
  affectedDependents: PluginStatus[];
};
export type SessionInspection = {
  sessionId: string;
  receipt: unknown;
  compatibilityResolutions: unknown[];
};

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const value = (await response.json()) as T & { error?: string };
  if (!response.ok) throw new Error(value.error ?? `Request failed (${response.status})`);
  return value;
}

export function listPlugins(): Promise<PluginInspection> {
  return postJSON("/api/plugins/list", {});
}

export function listPresets(): Promise<{ presets: PresetInspection[] }> {
  return postJSON("/api/presets/list", {});
}

export async function validatePreset(id: string): Promise<PresetInspection> {
  const response = await fetch("/api/presets/validate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id }),
  });
  const value = (await response.json()) as PresetInspection & { error?: string };
  if (response.ok || response.status === 422) return value;
  throw new Error(value.error ?? `Request failed (${response.status})`);
}

export function changePlugin(id: string, enabled: boolean): Promise<LifecycleTransition> {
  return postJSON("/api/plugins/lifecycle", { id, enabled });
}

export function inspectSession(sessionId: string): Promise<SessionInspection> {
  return postJSON("/api/sessions/inspect", { sessionId });
}
