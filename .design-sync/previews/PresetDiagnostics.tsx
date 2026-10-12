import { PresetDiagnostics } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const standard = { id: "standard", version: "1", valid: true, immutable: true, requiredCapabilities: [], optionalCapabilities: [], warnings: [], errors: [] };
const warnings = [
  "Optional capability finance.sync is unavailable: the finance plugin is disabled.",
  "Optional capability web.search is unavailable: no search provider is configured.",
  "Optional capability memory.review is unavailable: requires memory >=2.0, found 1.4.",
];
const errors = [
  "Required capability web.search is unavailable: no search provider is configured.",
  "Required capability web.read is unavailable: requires web >=2.0, found 1.1.",
];

// The diagnostics sit under the "Agent preset" select in Create workspace;
// the frame reproduces that field so the message reads in context.
const PresetField = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="border-hair-input bg-sidebar rounded-xl border px-6 py-5" style={{ maxWidth: 472 }}>
    <label className="text-body mb-2 block text-xs font-medium">Agent preset</label>
    <div className="border-hair-input bg-card text-ink w-full rounded-lg border px-3 py-2.5 text-[13px]">{label}</div>
    {children}
  </div>
);

// <details> is closed by default; the Expanded stories open it after mount.
const Expanded = ({ children }: { children: React.ReactNode }) => {
  const root = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => { const d = root.current?.querySelector("details"); if (d) d.open = true; }, []);
  return <div ref={root}>{children}</div>;
};

export const OptionalWarnings = () => (
  <AppFrame>
    <PresetField label="Standard">
      <p className="text-muted-text mt-2 text-xs leading-5">Evie’s full default toolset, using enabled plugins and normal approval rules.</p>
      <PresetDiagnostics preset={{ ...standard, warnings } as any} />
    </PresetField>
  </AppFrame>
);

export const OptionalWarningsExpanded = () => (
  <AppFrame>
    <PresetField label="Standard">
      <p className="text-muted-text mt-2 text-xs leading-5">Evie’s full default toolset, using enabled plugins and normal approval rules.</p>
      <Expanded><PresetDiagnostics preset={{ ...standard, warnings } as any} /></Expanded>
    </PresetField>
  </AppFrame>
);

export const Unavailable = () => (
  <AppFrame>
    <PresetField label="Research — unavailable">
      <p className="text-muted-text mt-2 text-xs leading-5">Web search and page reading only. No memory, local files, shell, or other plugins.</p>
      <PresetDiagnostics preset={{ ...standard, id: "research", valid: false, errors } as any} />
    </PresetField>
  </AppFrame>
);

export const UnavailableExpanded = () => (
  <AppFrame>
    <PresetField label="Research — unavailable">
      <p className="text-muted-text mt-2 text-xs leading-5">Web search and page reading only. No memory, local files, shell, or other plugins.</p>
      <Expanded><PresetDiagnostics preset={{ ...standard, id: "research", valid: false, errors } as any} /></Expanded>
    </PresetField>
  </AppFrame>
);

export const MissingPreset = () => (
  <AppFrame>
    <PresetField label="Standard — unavailable">
      <p className="text-muted-text mt-2 text-xs leading-5">Choose which plugin capabilities this workspace can use.</p>
      <PresetDiagnostics />
    </PresetField>
  </AppFrame>
);
