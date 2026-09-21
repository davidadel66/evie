import { useEffect, useId, useRef, type ReactNode } from "react";
import { Cross } from "./Icon";

type DialogProps = {
  title: string;
  description?: string;
  onClose: () => void;
  busy?: boolean;
  children: ReactNode;
  className?: string;
};

export function Dialog({ title, description, onClose, busy = false, children, className = "" }: DialogProps) {
  const dialog = useRef<HTMLDialogElement>(null);
  const backdropPointerDown = useRef(false);
  const titleId = useId();
  const descriptionId = useId();

  useEffect(() => {
    const element = dialog.current;
    const previousFocus = document.activeElement;
    // Native modal dialogs make the rest of the page inert and contain focus.
    element?.showModal();
    element?.querySelector<HTMLElement>("[data-dialog-autofocus]")?.focus({ preventScroll: true });
    return () => {
      element?.close();
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) {
        previousFocus.focus({ preventScroll: true });
      }
    };
  }, []);

  const close = () => { if (!busy) onClose(); };
  return (
    <dialog
      ref={dialog}
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      aria-busy={busy || undefined}
      onCancel={(event) => { event.preventDefault(); close(); }}
      onPointerDown={(event) => { backdropPointerDown.current = isBackdropPointer(event); }}
      onPointerCancel={() => { backdropPointerDown.current = false; }}
      onClick={(event) => {
        const clickedBackdrop = backdropPointerDown.current && isBackdropPointer(event);
        backdropPointerDown.current = false;
        if (clickedBackdrop) close();
      }}
      className={`border-hair-input bg-sidebar text-body fixed inset-0 m-auto max-h-[calc(100dvh_-_2rem)] w-[calc(100%_-_2rem)] max-w-[520px] overflow-x-hidden overflow-y-auto rounded-xl border p-0 shadow-2xl backdrop:bg-black/60 ${className}`}
    >
      <div>
        <header className="flex items-start justify-between gap-5 px-6 pt-6 pb-5">
          <div className="min-w-0">
            <h2 id={titleId} className="text-ink text-xl font-semibold tracking-[-0.02em]">{title}</h2>
            {description && <p id={descriptionId} className="text-muted-text mt-2 text-[13px] leading-5">{description}</p>}
          </div>
          <button type="button" aria-label={`Close ${title.toLowerCase()}`} disabled={busy} onClick={close} className="text-muted-text hover:bg-hover hover:text-ink focus-visible:ring-teal -mr-2 -mt-1 rounded-md p-2 focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">
            <Cross size={15} />
          </button>
        </header>
        <div className="px-6 pb-6">{children}</div>
      </div>
    </dialog>
  );
}

function isBackdropPointer(event: { target: EventTarget; currentTarget: HTMLDialogElement; clientX: number; clientY: number }) {
  if (event.target !== event.currentTarget) return false;
  const rect = event.currentTarget.getBoundingClientRect();
  return event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom;
}
