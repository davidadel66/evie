import type { Database } from "bun:sqlite"
import { appendFileSync, existsSync, mkdirSync, readdirSync, readFileSync, realpathSync, statSync } from "node:fs"
import { mkdir, writeFile } from "node:fs/promises"
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path"
import { runGit, runGitSync } from "./git.ts"
import { NOT_ACTIVITY } from "./log.ts"
import type { Project, ProjectColour } from "./types.ts"

// Projects and the sessions that belong to them. A project is found by its
// id; its folder is only where its files are today.
export type Projects = {
  // The project a folder belongs to, created the first time it is asked
  // for; or none, saying why, when the project's folder could never be one
  // (see `refusal`).
  forFolder(folder: string): Project | { refused: string }
  // Why a project's folder can never be a project's, or undefined when it
  // can: it is evie's data folder, is inside it or holds it, or its .evie
  // folder leads to one of those through a link. A project at the home
  // folder, which holds the data folder's usual place, would take evie's
  // own settings file there, ~/.evie/evie.json, for the project's own. None
  // is made at such a folder, and one an earlier evie made there is no
  // project now: its sessions open with no project.
  refusal(project: Project): string | undefined
  // Registers a new project rooted at exactly this folder, which must be a
  // full path with links resolved. Throws when the folder is already a project.
  create(folder: string, name: string): Project
  get(id: string): Project | undefined
  // Every project, by name.
  list(): Project[]
  // The icon and the colour the owner picked for each project that has
  // them, by the project's id.
  icons(): Map<string, { icon: string; colour: ProjectColour }>
  // Keeps the icon and the colour the owner picked for a project. Throws,
  // saying why, for a project that is not there, for an icon that is not
  // named as Phosphor names its icons, and for a colour not one of the six.
  setIcon(project: string, icon: string, colour: ProjectColour): void
  // Starts a session in a project, or the general session when there is
  // none. With `parent`, it is a subagent of that session.
  // `as` says which named agent a subagent is, and whether it is held to
  // reading only.
  createSession(project: Project | null, parent?: string, as?: { agent?: string; readOnly?: boolean }): string
  // The session used last in a project, or the last general session: the
  // one with the newest event, settling, naming and moving aside. Undefined when
  // none has had a conversation. A subagent's session is never the answer:
  // it is not the owner's to carry on.
  lastSession(project: Project | null): string | undefined
  // The project a session belongs to, null for a general session, or
  // undefined when there is no session with that id. A session whose
  // project is missing from the database, which only a damaged one holds,
  // is an error, not a general session.
  projectOf(session: string): Project | null | undefined
  // The session that started this one as a subagent, if one did.
  parentOf(session: string): string | undefined
  // What a subagent's session was started as.
  startedAs(session: string): { agent?: string; readOnly: boolean }
  // The subagents a session has started, newest first: each one's session,
  // when it was made, and the text of its task, its first message, or null
  // before it has one.
  subagentsOf(session: string): { id: string; created: number; task: string | null }[]
  // Puts a session and its subagents in another project, or with null in none.
  move(session: string, project: string | null): void
  // Takes a session away, which is being deleted.
  remove(session: string): void
}

// The folder a project is rooted at: the nearest folder at or above this one
// that holds a git repository, or the folder itself when there is none.
function projectRoot(folder: string): string {
  for (let current = folder; ; current = dirname(current)) {
    if (existsSync(join(current, ".git"))) return current
    if (dirname(current) === current) return folder
  }
}

// Keeps a project's `.evie/` folder out of the project's own git, when its
// folder is a git repository: `/.evie/` is added to the repository's local
// exclude list, which git reads as it reads a .gitignore but which is never
// committed. A worktree's `.git` is a file naming its own folder in the main
// repository, and a `commondir` file there names the folder whose exclude
// list it shares. A repository that already commits something in `.evie/`
// does so on purpose, and is left alone; when git cannot be asked, the line
// is added, since it never changes what is already committed. Nothing here
// may stop a project opening, so a list that cannot be read or written is
// left as it is.
export function keepEvieOutOfGit(folder: string): void {
  try {
    const dotGit = join(folder, ".git")
    const kind = statSync(dotGit, { throwIfNoEntry: false })
    if (!kind) return
    let gitFolder = dotGit
    if (!kind.isDirectory()) {
      const named = /^gitdir:\s*(.+?)\s*$/m.exec(readFileSync(dotGit, "utf8"))?.[1]
      if (!named) return
      gitFolder = resolve(folder, named)
      const common = join(gitFolder, "commondir")
      if (existsSync(common)) gitFolder = resolve(gitFolder, readFileSync(common, "utf8").trim())
    }
    const exclude = join(gitFolder, "info", "exclude")
    const listed = existsSync(exclude) ? readFileSync(exclude, "utf8") : ""
    if (listed.split(/\r?\n/).some((line) => line.trim() === EXCLUDED)) return
    if (tracksEvie(folder)) return
    mkdirSync(dirname(exclude), { recursive: true })
    appendFileSync(exclude, `${listed === "" || listed.endsWith("\n") ? "" : "\n"}${EXCLUDED}\n`)
  } catch {}
}
const EXCLUDED = "/.evie/"

// Whether the repository at this folder commits anything in its `.evie/`.
// Git runs none of the commands a repository's settings can name (git.ts).
function tracksEvie(folder: string): boolean {
  const asked = runGitSync(folder, ["ls-files", "--", ".evie"])
  return asked.code === 0 && asked.stdout.toString().trim() !== ""
}

// Whether a project's folder is in a git work tree, at its top or below it,
// as git itself says: true or false, or undefined when git cannot be asked
// or does not answer within a second. A folder with no `.git` in it or
// above it is not, and git is not asked. Git runs none of the commands a
// repository's settings can name, nor its own settings in the server's
// environment (git.ts).
export async function inGit(folder: string): Promise<boolean | undefined> {
  if (!existsSync(folder)) return false
  let above = folder
  while (!existsSync(join(above, ".git"))) {
    if (dirname(above) === above) return false
    above = dirname(above)
  }
  const asked = await runGit(folder, ["rev-parse", "--is-inside-work-tree"], { timeout: 1000 })
  if (asked.code === 0) return asked.stdout.toString().trim() === "true"
  return asked.stderr.includes("not a git repository") ? false : undefined
}

// The git branch a project's folder is on, as git itself says, or
// undefined when the folder is not the top of a repository, as one inside
// another repository is not, is on no branch, as with a commit checked out
// by itself, or git cannot be asked. A repository with no commits yet is on
// its branch all the same. Git is asked about the folder alone, as git.ts
// runs it: git's own settings in the server's environment, such as
// GIT_DIR, which would point it elsewhere, are left out. It is one small
// process, given up on after a second, so that a list of projects is never
// held up by one of them.
export async function branchOf(folder: string): Promise<string | undefined> {
  if (!existsSync(join(folder, ".git"))) return undefined
  const asked = await runGit(folder, ["symbolic-ref", "--short", "-q", "HEAD"], { timeout: 1000 })
  const branch = asked.stdout.toString().trim()
  return asked.code === 0 && branch !== "" ? branch : undefined
}

// A folder as it really is, links resolved, whether or not it exists yet:
// the part that exists is resolved and the rest added to it.
function realFolder(folder: string): string {
  const parent = dirname(folder)
  if (existsSync(folder) || parent === folder) return realpathSync(folder)
  return join(realFolder(parent), basename(folder))
}

// Whether a folder is another one or inside it. A folder inside it may
// have a name that starts with two dots, such as `..cache`.
const within = (folder: string, outer: string) => {
  const path = relative(outer, folder)
  return path !== ".." && !path.startsWith(`..${sep}`) && !isAbsolute(path)
}

// How a folder stands to evie's data folder, both as they really are: it is
// the data folder, is inside it, or holds it; undefined when it is none of
// these, as a project's folder must be.
function besideData(folder: string, data: string): "is" | "inside" | "holds" | undefined {
  if (folder === data) return "is"
  if (within(folder, data)) return "inside"
  if (within(data, folder)) return "holds"
  return undefined
}

// Why a folder, as it really is, can never be a project's (see `refusal`),
// in words, or undefined when it can. `data` is the data folder as it
// really is, and `shown` as evie was given it.
function whyNotAProject(folder: string, data: string, shown: string): string | undefined {
  const stands = { is: "is evie's data folder", inside: `is inside evie's data folder, ${shown}`, holds: `holds evie's data folder, ${shown}` }
  const how = besideData(folder, data)
  if (how !== undefined) return `${folder} ${stands[how]}, where evie keeps its own settings and state, so it is never made a project.`
  const evie = realFolder(join(folder, ".evie"))
  const through = besideData(evie, data)
  if (through === undefined) return undefined
  return `The .evie folder of ${folder} leads to ${evie}, which ${stands[through]}, where evie keeps its own settings and state, so ${folder} is never made a project.`
}

// What says to give a folder inside a repository that is no project's a
// repository of its own, so that it is a project's itself.
const ownRepository = (folder: string) => `To make ${folder} a project of its own, give it a git repository of its own, with git init there.`

// Makes a new project, as the `new_project` command asks: the folder when it
// is missing, and its `.evie/evie.json`, which backs up its `.evie/` folder
// to a private repository, unless it is asked to keep it on this computer.
// It is registered at the folder as it really is.
// Everything that would refuse it is checked before anything is made.
// `heldFiles` says that its `.evie/` folder already held something, such as
// skills or agents, which came with the folder and so are not to be trusted
// without the owner reading them.
export async function makeProject(
  projects: Projects,
  input: { folder: string; name?: string; sync?: "private" | "none" },
  dataFolder: string,
  homeFolder: string,
): Promise<{ project: Project; madeFolder: boolean; heldFiles: boolean; sync: "private" | "none" }> {
  // Checked here, since a client that is not type-checked can send anything.
  // A folder in the home folder may be written from ~, as a shell reads it.
  const given = typeof input.folder === "string" ? input.folder.replace(/^~(?=\/|$)/, homeFolder) : input.folder
  if (typeof given !== "string" || !isAbsolute(given)) {
    throw new Error(`${JSON.stringify(input.folder)} is not a full path. Give the folder's full path, such as /Users/me/finances, or one in your home folder, such as ~/finances.`)
  }
  // A name of only dots, such as "..", names no folder of its own.
  if (input.name !== undefined && (typeof input.name !== "string" || /^\.*$/.test(input.name.trim()))) {
    throw new Error("`name` is the project's name, and must be text that is not blank and not only dots, such as Finances.")
  }
  const sync = input.sync ?? "private"
  if (sync !== "private" && sync !== "none") {
    throw new Error('`sync` is where the project\'s .evie folder is backed up: "private", to a private repository, or "none", to keep it on this computer.')
  }
  if (existsSync(given) && !statSync(given).isDirectory()) throw new Error(`${given} is a file, not a folder.`)
  const folder = realFolder(given)
  const data = realFolder(dataFolder)
  if (within(folder, data)) throw new Error(`${given} is in evie's data folder, ${dataFolder}, where evie keeps its own state. Choose a folder outside it.`)
  // As it really is, since a .evie that is a link counts where it leads.
  const ownEvie = besideData(realFolder(join(folder, ".evie")), data)
  if (ownEvie === "is" || ownEvie === "holds") throw new Error(`The .evie folder of ${given} is, or holds, evie's data folder, ${dataFolder}. Choose another folder for the project.`)
  if (ownEvie === "inside") throw new Error(`The .evie folder of ${given} leads into evie's data folder, ${dataFolder}. Choose another folder for the project.`)
  if (besideData(folder, data) === "holds") throw new Error(`${given} holds evie's data folder, ${dataFolder}, where evie keeps its own state. Choose a folder that does not hold it.`)
  // Opened later, a folder inside a repository is taken to be the
  // repository's project, so a project of its own there could not be
  // reached. Checked after the folder itself, so that a folder offered a
  // repository of its own could be a project.
  const root = projectRoot(folder)
  const rootRefused = root === folder ? undefined : whyNotAProject(root, data, dataFolder)
  if (rootRefused !== undefined) {
    throw new Error(`${given} is inside the git repository at ${root}, which is never a project: ${rootRefused} ${ownRepository(given)}`.replace(/\.$/, ", then make the project."))
  }
  if (root !== folder) {
    throw new Error(`${given} is inside the git repository at ${root}, so it belongs to that repository's project. Make the project at the repository's own folder instead.`)
  }
  const evie = join(folder, ".evie")
  if (existsSync(evie) && !statSync(evie).isDirectory()) {
    throw new Error(`${evie} is a file. evie keeps what it adds to a project in a folder of that name, so move the file elsewhere, then make the project.`)
  }
  const settingsFile = join(evie, "evie.json")
  const already = projects.list().find((project) => project.folder === folder)
  if (already) throw new Error(`${given} is already the project "${already.name}". To back up its .evie folder, set "sync": "private" in ${settingsFile}.`)
  // Settings that came with the folder, as in a repository cloned from
  // someone else, are not to be trusted without the owner reading them.
  if (existsSync(settingsFile)) {
    throw new Error(`${given} already has evie settings of its own, ${settingsFile}. Open a session there with \`evie\` instead, which asks whether to trust them.`)
  }

  const madeFolder = !existsSync(folder)
  const heldFiles = existsSync(evie) && readdirSync(evie).length > 0
  await mkdir(evie, { recursive: true })
  await writeFile(settingsFile, `${JSON.stringify({ sync }, null, 2)}\n`, { flag: "wx" })
  return { project: projects.create(folder, input.name?.trim() ?? basename(folder)), madeFolder, heldFiles, sync }
}

// The six project colours, as the design system names them.
const PROJECT_COLOURS: readonly ProjectColour[] = ["moss", "mustard", "maroon", "rust", "slate", "plum"]
// An icon's name as Phosphor writes it: words of lower-case letters and
// digits, joined by dashes, such as "cooking-pot".
const ICON_NAME = /^[a-z0-9]+(?:-[a-z0-9]+)*$/
const LONGEST_ICON_NAME = 64

// `dataFolder` is evie's data folder, which no project's folder may be,
// be inside or hold.
export function createProjects(db: Database, dataFolder: string): Projects {
  const byFolder = db.query<Project, [string]>("SELECT id, name, folder FROM project WHERE folder = ?")
  const byId = db.query<Project, [string]>("SELECT id, name, folder FROM project WHERE id = ?")
  const all = db.query<Project, []>("SELECT id, name, folder FROM project ORDER BY name COLLATE NOCASE, created")
  const insertProject = db.query("INSERT INTO project (id, name, folder, created) VALUES (?, ?, ?, ?)")
  const picked = db.query<{ id: string; icon: string; colour: string }, []>("SELECT id, icon, colour FROM project WHERE icon IS NOT NULL AND colour IS NOT NULL")
  const updateIcon = db.query("UPDATE project SET icon = ?, colour = ? WHERE id = ?")
  const insertSession = db.query("INSERT INTO session (id, project, created, parent, agent, read_only) VALUES (?, ?, ?, ?, ?, ?)")
  const sessionRow = db.query<{ project: string | null; parent: string | null; agent: string | null; read_only: number | null }, [string]>(
    "SELECT project, parent, agent, read_only FROM session WHERE id = ?",
  )
  const children = db.query<{ id: string; created: number; first: string | null }, [string]>(
    `SELECT id, created, (SELECT body FROM event WHERE event.session = session.id AND type = 'user_message' ORDER BY seq LIMIT 1) AS first
     FROM session WHERE parent = ? ORDER BY created DESC, rowid DESC`,
  )
  // `IS` compares with null as well as with an id. Only a session the owner
  // has said something in counts: one that holds nothing but a question
  // asked while it was opening is not a conversation to carry on. Settling
  // a session by hand, or bringing it back, naming it and moving it are not
  // using it, so they are left out, as they are from a session's activity.
  const lastUsed = db.query<{ id: string }, [string | null]>(
    `SELECT session.id FROM session JOIN event ON event.session = session.id AND NOT ${NOT_ACTIVITY}
     WHERE session.project IS ?
       AND session.parent IS NULL
       AND session.id IN (SELECT session FROM event WHERE type = 'user_message')
     GROUP BY session.id
     ORDER BY MAX(event.time) DESC, session.created DESC
     LIMIT 1`,
  )

  const moveRows = db.query("UPDATE session SET project = ? WHERE id = ? OR parent = ?")
  const removeRow = db.query("DELETE FROM session WHERE id = ?")

  const insert = (folder: string, name: string): Project => {
    const project: Project = { id: crypto.randomUUID(), name, folder }
    insertProject.run(project.id, project.name, project.folder, Date.now())
    return project
  }
  const findOrCreate = db.transaction((root: string): Project => byFolder.get(root) ?? insert(root, basename(root)))
  const create = db.transaction((folder: string, name: string): Project => {
    const existing = byFolder.get(folder)
    if (existing) throw new Error(`${folder} is already the project "${existing.name}".`)
    return insert(folder, name)
  })

  return {
    forFolder(folder) {
      if (!existsSync(folder)) throw new Error(`The folder "${folder}" does not exist.`)
      if (!statSync(folder).isDirectory()) throw new Error(`"${folder}" is a file, not a folder.`)
      // Symbolic links are resolved so one folder is never two projects, and
      // so a link cannot pass the data folder off as another folder.
      const resolved = realpathSync(folder)
      const root = projectRoot(resolved)
      const data = realFolder(dataFolder)
      const why = whyNotAProject(root, data, dataFolder)
      if (why === undefined) return findOrCreate(root)
      if (root === resolved) return { refused: why }
      // A folder inside a repository that is no project's, as a home folder
      // kept in git is, is a project's once it has a repository of its own,
      // unless it could not be one either.
      const offer = whyNotAProject(resolved, data, dataFolder) === undefined ? ` ${ownRepository(resolved)}` : ""
      return { refused: `${resolved} belongs to the git repository at ${root}. ${why}${offer}` }
    },

    refusal(project) {
      return whyNotAProject(realFolder(project.folder), realFolder(dataFolder), dataFolder)
    },

    create(folder, name) {
      return create(folder, name)
    },

    get(id) {
      return byId.get(id) ?? undefined
    },

    list() {
      return all.all()
    },

    icons() {
      // A colour this Evie does not know, as one a later Evie could add,
      // is left out with its icon, so the project is drawn as before.
      const known = picked.all().filter((row) => PROJECT_COLOURS.includes(row.colour as ProjectColour))
      return new Map(known.map((row) => [row.id, { icon: row.icon, colour: row.colour as ProjectColour }]))
    },

    setIcon(project, icon, colour) {
      // Checked here, since a client that is not type-checked can send anything.
      if (typeof icon !== "string" || icon.length > LONGEST_ICON_NAME || !ICON_NAME.test(icon)) {
        throw new Error(`${JSON.stringify(icon)} is not the name of an icon. Give a Phosphor icon's name, in lower case with dashes, such as "folder" or "cooking-pot".`)
      }
      if (!PROJECT_COLOURS.includes(colour)) {
        throw new Error(`${JSON.stringify(colour)} is not one of the project colours. Give one of: ${PROJECT_COLOURS.join(", ")}.`)
      }
      if (typeof project !== "string" || !byId.get(project)) throw new Error(`There is no project with the id ${JSON.stringify(project)}.`)
      updateIcon.run(icon, colour, project)
    },

    createSession(project, startedBy, as) {
      const id = crypto.randomUUID()
      insertSession.run(id, project?.id ?? null, Date.now(), startedBy ?? null, as?.agent ?? null, as?.readOnly ? 1 : 0)
      return id
    },

    startedAs(session) {
      const row = sessionRow.get(session)
      return { ...(row?.agent != null && { agent: row.agent }), readOnly: row?.read_only === 1 }
    },

    projectOf(session) {
      const row = sessionRow.get(session)
      if (!row) return undefined
      if (row.project === null) return null
      const project = byId.get(row.project)
      if (!project) throw new Error(`The session "${session}" belongs to the project "${row.project}", which is missing from evie's database.`)
      return project
    },

    parentOf(session) {
      return sessionRow.get(session)?.parent ?? undefined
    },

    lastSession(project) {
      return lastUsed.get(project?.id ?? null)?.id
    },

    subagentsOf(session) {
      return children.all(session).map(({ id, created, first }) => ({ id, created, task: textOf(first) }))
    },

    move(session, project) {
      moveRows.run(project, session, session)
    },

    remove(session) {
      removeRow.run(session)
    },
  }
}

// The text of a stored message's body, or null when it has none, as a
// damaged database could hold.
function textOf(body: string | null): string | null {
  if (body === null) return null
  try {
    const parsed: unknown = JSON.parse(body)
    const text = typeof parsed === "object" && parsed !== null ? (parsed as { text?: unknown }).text : undefined
    return typeof text === "string" ? text : null
  } catch {
    return null
  }
}
