import type { Database } from "bun:sqlite"
import { appendFileSync, existsSync, mkdirSync, readFileSync, realpathSync, statSync } from "node:fs"
import { mkdir, writeFile } from "node:fs/promises"
import { basename, dirname, isAbsolute, join, relative, resolve } from "node:path"
import type { Project } from "./types.ts"

// Projects and the sessions that belong to them. A project is found by its
// id; its folder is only where its files are today.
export type Projects = {
  // The project a folder belongs to, created the first time it is asked for.
  forFolder(folder: string): Project
  // Registers a new project rooted at exactly this folder, which must be a
  // full path with links resolved. Throws when the folder is already a project.
  create(folder: string, name: string): Project
  get(id: string): Project | undefined
  // Every project, by name.
  list(): Project[]
  // Starts a session in a project, or the general session when there is
  // none. With `parent`, it is a subagent of that session.
  // `as` says which named agent a subagent is, and whether it is held to
  // reading only.
  createSession(project: Project | null, parent?: string, as?: { agent?: string; readOnly?: boolean }): string
  // The session used last in a project, or the last general session: the one
  // with the newest event, settling aside. Undefined when none has had a
  // conversation. A subagent's session is never the answer: it is not the
  // owner's to carry on.
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
// list it shares. Nothing here may stop a project opening, so a list that
// cannot be read or written is left as it is.
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
    mkdirSync(dirname(exclude), { recursive: true })
    appendFileSync(exclude, `${listed === "" || listed.endsWith("\n") ? "" : "\n"}${EXCLUDED}\n`)
  } catch {}
}
const EXCLUDED = "/.evie/"

// A folder as it really is, links resolved, whether or not it exists yet:
// the part that exists is resolved and the rest added to it.
function realFolder(folder: string): string {
  const parent = dirname(folder)
  if (existsSync(folder) || parent === folder) return realpathSync(folder)
  return join(realFolder(parent), basename(folder))
}

// Whether a folder is another one or inside it.
const within = (folder: string, outer: string) => {
  const path = relative(outer, folder)
  return path === "" || (!path.startsWith("..") && !isAbsolute(path))
}

// Makes a new project, as the `new_project` command asks: the folder when it
// is missing, and its `.evie/evie.json`, which backs up its `.evie/` folder
// to a private repository. It is registered at the folder as it really is.
// Everything that would refuse it is checked before anything is made.
export async function makeProject(projects: Projects, input: { folder: string; name?: string }, dataFolder: string): Promise<{ project: Project; madeFolder: boolean }> {
  // Checked here, since a client that is not type-checked can send anything.
  const given = input.folder
  if (typeof given !== "string" || !isAbsolute(given)) {
    throw new Error(`${JSON.stringify(given)} is not a full path. Give the folder's full path, such as /Users/me/finances.`)
  }
  if (input.name !== undefined && (typeof input.name !== "string" || input.name.trim() === "")) {
    throw new Error("`name` is the project's name, and must be text that is not blank.")
  }
  if (existsSync(given) && !statSync(given).isDirectory()) throw new Error(`${given} is a file, not a folder.`)
  const folder = realFolder(given)
  const data = realFolder(dataFolder)
  // Opened later, a folder inside a repository is taken to be the
  // repository's project, so a project of its own there could not be reached.
  const root = projectRoot(folder)
  if (root !== folder) {
    throw new Error(`${given} is inside the git repository at ${root}, so it belongs to that repository's project. Make the project at the repository's own folder instead.`)
  }
  if (within(folder, data)) throw new Error(`${given} is in Evie's data folder, ${dataFolder}, where Evie keeps its own state. Choose a folder outside it.`)
  if (within(data, join(folder, ".evie"))) throw new Error(`The .evie folder of ${given} is, or holds, Evie's data folder, ${dataFolder}. Choose another folder for the project.`)
  const settingsFile = join(folder, ".evie", "evie.json")
  const already = projects.list().find((project) => project.folder === folder)
  if (already) throw new Error(`${given} is already the project "${already.name}". To back up its .evie folder, set "sync": "private" in ${settingsFile}.`)
  // Settings that came with the folder, as in a repository cloned from
  // someone else, are not to be trusted without the owner reading them.
  if (existsSync(settingsFile)) {
    throw new Error(`${given} already has Evie settings of its own, ${settingsFile}. Open a session there with \`evie\` instead, which asks whether to trust them.`)
  }

  const madeFolder = !existsSync(folder)
  await mkdir(join(folder, ".evie"), { recursive: true })
  await writeFile(settingsFile, `${JSON.stringify({ sync: "private" }, null, 2)}\n`, { flag: "wx" })
  return { project: projects.create(folder, input.name?.trim() ?? basename(folder)), madeFolder }
}

export function createProjects(db: Database): Projects {
  const byFolder = db.query<Project, [string]>("SELECT id, name, folder FROM project WHERE folder = ?")
  const byId = db.query<Project, [string]>("SELECT id, name, folder FROM project WHERE id = ?")
  const all = db.query<Project, []>("SELECT id, name, folder FROM project ORDER BY name COLLATE NOCASE, created")
  const insertProject = db.query("INSERT INTO project (id, name, folder, created) VALUES (?, ?, ?, ?)")
  const insertSession = db.query("INSERT INTO session (id, project, created, parent, agent, read_only) VALUES (?, ?, ?, ?, ?, ?)")
  const sessionRow = db.query<{ project: string | null; parent: string | null; agent: string | null; read_only: number | null }, [string]>(
    "SELECT project, parent, agent, read_only FROM session WHERE id = ?",
  )
  // `IS` compares with null as well as with an id. Only a session the owner
  // has said something in counts: one that holds nothing but a question
  // asked while it was opening is not a conversation to carry on. Settling
  // a session by hand, or bringing it back, is not using it, so it is left
  // out, as it is from a session's activity.
  const lastUsed = db.query<{ id: string }, [string | null]>(
    `SELECT session.id FROM session JOIN event ON event.session = session.id AND event.type != 'settled'
     WHERE session.project IS ?
       AND session.parent IS NULL
       AND session.id IN (SELECT session FROM event WHERE type = 'user_message')
     GROUP BY session.id
     ORDER BY MAX(event.time) DESC, session.created DESC
     LIMIT 1`,
  )

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
      // Symbolic links are resolved so one folder is never two projects.
      const resolved = realpathSync(folder)
      return findOrCreate(projectRoot(resolved))
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
      if (!project) throw new Error(`The session "${session}" belongs to the project "${row.project}", which is missing from Evie's database.`)
      return project
    },

    parentOf(session) {
      return sessionRow.get(session)?.parent ?? undefined
    },

    lastSession(project) {
      return lastUsed.get(project?.id ?? null)?.id
    },
  }
}
