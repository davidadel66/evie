// The System view (system-page.spec.md): the system map page, built into the
// binary and served at /explorer/, full height inside Evie and always dark.
// The page draws its own Map, Architecture and History tabs and routes
// between them itself, so the view adds none (system-page.decisions.md,
// 2026-11-05). Like Data, the view is not remembered: the page opens on its
// subway map every time. The frame has scripts and popups but not
// same-origin, so the page cannot use Evie's API or storage; popups escape
// the sandbox so History's GitHub links open normally.

const systemPage = "/explorer/?theme=dark";

export function SystemView() {
  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <header className="border-hair flex min-w-0 flex-none items-end gap-3 border-b px-5 pt-4 sm:gap-8 sm:px-7">
        <div className="pb-3">
          <h1 className="text-ink text-[19px] font-semibold tracking-[-0.02em]">System</h1>
        </div>
      </header>
      <iframe
        title="System map"
        src={systemPage}
        sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
        referrerPolicy="no-referrer"
        className="bg-app min-h-0 w-full flex-1 border-0"
      />
    </main>
  );
}
