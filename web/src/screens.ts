import { settingsIn } from "./address.ts"
import { cardShown } from "./card-shown.ts"
import { later, lazyFrom } from "./later.ts"
import { workspaceIn } from "./workspace.ts"

// The screens the page opens rarely or later, which its first script does
// not carry (see later.ts): the settings; the command palette; the session
// details, whose place on the page is CardPlace.tsx; the right panel, with
// its diff; the workspace view, with its side chat and its storage; and
// the frames and viewers of the files evie shows (ShownLater.tsx).
// They are one file, which the page fetches once it is idle, so that each
// opens at once the first time, or at once when it opens on one of them.
// One file rather than one each: the bundler gives the code each set of
// files shares a file of its own, and five would split what the page
// loads first into some twenty files, a round trip more each on a phone.
export const SCREENS = later(() => import("./screens-later.ts"))

export const SettingsPage = lazyFrom(SCREENS, (module) => module.SettingsPage)
export const Palette = lazyFrom(SCREENS, (module) => module.Palette)
export const DetailsCard = lazyFrom(SCREENS, (module) => module.DetailsCard)
export const RightPanel = lazyFrom(SCREENS, (module) => module.RightPanel)
export const WorkspaceView = lazyFrom(SCREENS, (module) => module.WorkspaceView)
export const ShownFrame = lazyFrom(SCREENS, (module) => module.ShownFrame)

// Fetches the screens at once when the page opens on one, beside its
// connection to the server, so that it is here by the time the page is
// drawn: the session details while they are pinned open, and the workspace
// view or the settings when the address is theirs.
export function loadScreensOpenAtStart() {
  if (cardShown().open || workspaceIn(location.hash) !== undefined || settingsIn(location.hash) !== undefined) SCREENS.preload()
}
