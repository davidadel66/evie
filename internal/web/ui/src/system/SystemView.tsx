// The System view (system-page.spec.md): the system explorer, built into the
// binary and served at /explorer/, full height inside Evie. The frame has
// scripts and popups but not same-origin, so the explorer cannot use Evie's
// API or storage; popups escape the sandbox so GitHub links open normally.
export function SystemView() {
  return (
    <iframe
      title="System explorer"
      src="/explorer/?embedded=1"
      sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
      referrerPolicy="no-referrer"
      className="bg-app min-h-0 w-full flex-1 border-0"
    />
  );
}
