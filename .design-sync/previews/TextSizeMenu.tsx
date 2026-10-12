import { TextSizeMenu } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
const noop = () => {};

// The menu opens from internal state on a trigger click. The open stories
// click the trigger once after mount so the popover renders deterministically.
const OpenAfterMount = ({ children }: { children: React.ReactNode }) => {
  const root = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => { root.current?.querySelector<HTMLButtonElement>("button[aria-controls]")?.click(); }, []);
  return <div ref={root}>{children}</div>;
};

// The trigger as it sits in the chat header, collapsed.
export const Trigger = () => (
  <AppFrame>
    <div className="flex h-[54px] items-center justify-end gap-2 px-2">
      <span className="text-faint text-[11px]">Finance · Credit card payment matching</span>
      <TextSizeMenu value="default" onChange={noop} />
    </div>
  </AppFrame>
);

export const OpenBelow = () => (
  <AppFrame>
    <OpenAfterMount>
      <div className="flex items-start justify-end pr-2" style={{ height: 210 }}>
        <TextSizeMenu value="default" onChange={noop} placement="bottom" />
      </div>
    </OpenAfterMount>
  </AppFrame>
);

export const OpenAbove = () => (
  <AppFrame>
    <OpenAfterMount>
      <div className="flex items-end justify-end pr-2" style={{ height: 210 }}>
        <TextSizeMenu value="large" onChange={noop} placement="top" />
      </div>
    </OpenAfterMount>
  </AppFrame>
);

export const OpenCompact = () => (
  <AppFrame>
    <OpenAfterMount>
      <div className="flex items-start justify-end pr-2" style={{ height: 210 }}>
        <TextSizeMenu value="compact" onChange={noop} />
      </div>
    </OpenAfterMount>
  </AppFrame>
);
