import { CreateWorkspaceDialog } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
const noop = () => {};
const create = async () => {};

const standard = { id: "standard", version: "1", valid: true, immutable: true, requiredCapabilities: [], optionalCapabilities: [], warnings: [], errors: [] };
const research = { ...standard, id: "research" };
const degradedStandard = {
  ...standard,
  optionalCapabilities: [{ ID: "finance.sync", Compatibility: ">=1.0" }, { ID: "web.search", Compatibility: ">=2.0" }],
  warnings: ["Optional capability finance.sync is unavailable: the finance plugin is disabled.", "Optional capability web.search is unavailable: no search provider is configured."],
};

// The dialog loads presets from POST /api/presets/list on mount. The preview
// page has no Evie server, so the stories answer that one request themselves;
// which answer depends on the story that mounted.
let presetsAnswer: { presets?: unknown[]; error?: string } = { presets: [standard, research] };
const realFetch = window.fetch.bind(window);
window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
  const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
  if (url.endsWith("/api/presets/list")) {
    const failed = Boolean(presetsAnswer.error);
    return Promise.resolve(new Response(JSON.stringify(presetsAnswer), { status: failed ? 503 : 200, headers: { "Content-Type": "application/json" } }));
  }
  return realFetch(input, init);
}) as typeof window.fetch;

const Answer = ({ value, children }: { value: typeof presetsAnswer; children: React.ReactNode }) => {
  presetsAnswer = value;
  return <>{children}</>;
};

export const Ready = () => (
  <AppFrame>
    <Answer value={{ presets: [standard, research] }}>
      <CreateWorkspaceDialog onClose={noop} onCreate={create} />
    </Answer>
  </AppFrame>
);

export const OptionalCapabilitiesMissing = () => (
  <AppFrame>
    <Answer value={{ presets: [degradedStandard, research] }}>
      <CreateWorkspaceDialog onClose={noop} onCreate={create} />
    </Answer>
  </AppFrame>
);

export const ResearchUnavailable = () => (
  <AppFrame>
    <Answer value={{ presets: [standard, { ...research, valid: false, errors: ["Required capability web.search is unavailable: no search provider is configured."] }] }}>
      <CreateWorkspaceDialog onClose={noop} onCreate={create} />
    </Answer>
  </AppFrame>
);

export const PresetsFailedToLoad = () => (
  <AppFrame>
    <Answer value={{ error: "Agent presets could not be loaded: the plugin registry is still starting." }}>
      <CreateWorkspaceDialog onClose={noop} onCreate={create} />
    </Answer>
  </AppFrame>
);
