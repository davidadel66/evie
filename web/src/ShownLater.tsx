import { KindImage } from "./icons.tsx"
import { Screen } from "./later.ts"
import { SCREENS, ShownFrame } from "./screens.ts"
import type { Shown } from "./shown.ts"

// A file evie showed, in the conversation: its frame and viewer come with
// the screens the page loads later (screens.ts), which are fetched once
// the page is idle, so that the page's first script does not carry them.
// A file drawn before they are here asks for them, and until they come
// stands as an empty frame of its name and folder, so that the
// conversation keeps its place as the frame fills in.
export function ShownLater({ file }: { file: Shown }) {
  return (
    <Screen loads={SCREENS} wanted fallback={<Waiting file={file} />}>
      <ShownFrame file={file} />
    </Screen>
  )
}

function Waiting({ file }: { file: Shown }) {
  return (
    <figure aria-label={`Shown by evie: ${file.name}`} aria-busy="true" className="mt-1 rounded-md bg-text/4">
      <figcaption className="flex h-shown-header min-w-0 items-center gap-2 pr-1 pl-3 text-caption">
        <KindImage className="size-icon-row shrink-0 text-text-muted" />
        <span className="max-w-full shrink-0 truncate font-medium text-text">{file.name}</span>
        <span className="min-w-0 truncate font-mono text-text-muted">{file.folder}</span>
      </figcaption>
      <div className="mx-1 h-shown-svg pb-1" />
    </figure>
  )
}
