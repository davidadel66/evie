// The failure surface. Anything that goes wrong between browser and server —
// a dropped stream, a 409, a rejected origin — says so here rather than only
// in the console.
//
// The design's copy promised "retrying in 3s"; there is no auto-retry, and
// without a history endpoint there'd be nothing to reconnect to. Retry
// therefore just clears the banner and unblocks the composer.

import { Alert } from "./Icon";

type Props = { message: string; onDismiss: () => void };

export function Banner({ message, onDismiss }: Props) {
  return (
    <div className="border-danger-hair-soft bg-danger-banner flex flex-none items-center gap-[10px] border-b px-5 py-2">
      <Alert stroke="var(--color-danger)" />
      <span className="text-danger-ink-soft font-sans text-xs">{message}</span>
      <span
        onClick={onDismiss}
        className="cursor-pointer rounded-[5px] border border-[#3a2226] px-2 py-[2px] font-mono text-[11px] text-[#8b5a5a]"
      >
        Retry now
      </span>
      <div className="flex-1" />
      <span
        onClick={onDismiss}
        className="cursor-pointer text-[15px] leading-none text-[#8b5a5a]"
      >
        ×
      </span>
    </div>
  );
}
