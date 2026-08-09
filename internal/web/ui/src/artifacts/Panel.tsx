// The artifacts panel, shipped as chrome only: the rail, the header, and the
// empty state. Nothing emits artifacts until the whiteboard feature, so its
// card types land there rather than as placeholders here.

import { ChevronLeft, ChevronRight, FileIcon } from "../ui/Icon";

type Props = { open: boolean; onToggle: () => void };

export function Panel({ open, onToggle }: Props) {
  if (!open) {
    return (
      <div
        onClick={onToggle}
        title="Open artifacts"
        className="bg-panel flex w-[34px] flex-none cursor-pointer flex-col items-center gap-[10px] pt-[14px]"
      >
        <span className="text-faint">
          <ChevronLeft size={14} />
        </span>
        <span
          className="text-ghost font-mono text-[10px] font-semibold"
          style={{ writingMode: "vertical-rl" }}
        >
          ARTIFACTS · 0
        </span>
      </div>
    );
  }

  return (
    <div className="bg-panel flex min-h-0 w-[620px] flex-none flex-col">
      <div className="flex h-[42px] flex-none items-center gap-2 border-b border-[#1e2624] px-4">
        <span className="text-muted">
          <FileIcon size={14} />
        </span>
        <span className="text-body-dim font-sans text-[12.5px] font-semibold">
          Artifacts
        </span>
        <span className="text-ghost font-mono text-[10.5px]">
          this conversation · 0
        </span>
        <div className="flex-1" />
        <span onClick={onToggle} className="text-faint ml-1 cursor-pointer">
          <ChevronRight size={14} />
        </span>
      </div>
      <div className="text-ghost flex flex-1 flex-col items-center justify-center gap-2">
        <span className="text-[#37413e]">
          <FileIcon size={26} width={1.5} />
        </span>
        <span className="font-sans text-xs">Nothing pinned yet</span>
        <span className="font-sans text-[11px] text-[#37413e]">
          Evie pins figures, diagrams and images here as she works
        </span>
      </div>
    </div>
  );
}
