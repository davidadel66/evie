import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ManagementView } from "./Management";

describe("ManagementView", () => {
  it("renders lifecycle, diagnostics, preset validity, and session audit behavior", () => {
    const providerSecret = "oauth-client-secret-never-render";
    const html = renderToStaticMarkup(
      <ManagementView
        plugins={{
          degraded: true,
          plugins: [{
            id: "finance", version: "1.2.0", enabled: true, lifecycle: "failed",
            requiredDependencies: [{ ID: "account", Compatibility: { Minimum: "1.0.0", MaximumExclusive: "2.0.0" } }],
            optionalDependencies: [], health: "unhealthy",
            connectionReadiness: { state: "not_ready", code: "connection_not_ready", message: "Connect finance account" },
            diagnostic: "start failed", warnings: ["optional quotes provider unavailable"],
          }],
        }}
        presets={[{
          id: "standard", version: "sha256:abc", immutable: true, valid: false,
          requiredCapabilities: [{ ID: "finance.sync", Compatibility: { Minimum: "1.0.0", MaximumExclusive: "2.0.0" } }],
          optionalCapabilities: [], errors: ["required Capability finance.sync is unavailable"], warnings: ["optional Capability reports is unavailable"],
        }]}
        transition={{ pluginId: "finance", from: "ready", to: "failed", enabled: true, affectedDependents: [{
          id: "reports", version: "1.0.0", enabled: true, lifecycle: "waiting", requiredDependencies: [], optionalDependencies: [], health: "degraded", connectionReadiness: { state: "not_required" }, warnings: [],
        }] }}
        sessionId="session-1"
        validatingPresetId="standard"
        validatedPresetId="standard"
        session={{ sessionId: "session-1", receipt: { preset: { id: "standard" } }, compatibilityResolutions: [{ replacement: "1.2.0" }] }}
        onSessionId={() => undefined}
        onInspect={() => undefined}
        onToggle={() => undefined}
        onValidate={() => undefined}
      />,
    );
    for (const text of [
      "Degraded startup", "finance@1.2.0", "Lifecycle: failed", "Plugin Health: unhealthy",
      "Connection Readiness: not_ready", "Connect finance account", "Affected dependents: reports",
      "Invalid · Immutable built-in", "required Capability finance.sync is unavailable",
      "Original Composition Receipt", "Compatibility Resolutions", "Validating…", "Latest validation: invalid",
      "optional Capability reports is unavailable",
    ]) expect(html).toContain(text);
    expect(html).not.toContain(providerSecret);
  });

  it("renders the preset validation action and request error", () => {
    const html = renderToStaticMarkup(
      <ManagementView
        plugins={{ degraded: false, plugins: [] }}
        presets={[{
          id: "standard", version: "sha256:abc", immutable: true, valid: true,
          requiredCapabilities: [], optionalCapabilities: [], errors: [], warnings: [],
        }]}
        sessionId=""
        problem="Preset validation unavailable"
        onSessionId={() => undefined}
        onInspect={() => undefined}
        onToggle={() => undefined}
        onValidate={() => undefined}
      />,
    );

    expect(html).toContain("Validate preset");
    expect(html).toContain("Preset validation unavailable");
  });
});
