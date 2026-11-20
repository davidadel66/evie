import type { Database } from "bun:sqlite"
import { existsSync, realpathSync, statSync } from "node:fs"
import { basename, dirname, join } from "node:path"
import type { Project } from "./types.ts"

// Projects and the sessions that belong to them. A project is found by its
// id; its folder is only where its files are today.
export type Projects = {
  // The project a folder belongs to, created the first time it is asked for.
  forFolder(folder: string): Project
  get(id: string): Project | undefined
  // Starts a session in a project, or the general session when there is none.
  createSession(project: Project | null): string
  // The session used last in a project, or the last general session: the one
  // with the newest event. Undefined when none has had a conversation.
  lastSession(project: Project | null): string | undefined
}

// The folder a project is rooted at: the nearest folder at or above this one
// that holds a git repository, or the folder itself when there is none.
function projectRoot(folder: string): string {
  for (let current = folder; ; current = dirname(current)) {
    if (existsSync(join(current, ".git"))) return current
    if (dirname(current) === current) return folder
  }
}

export function createProjects(db: Database): Projects {
  const byFolder = db.query<Project, [string]>("SELECT id, name, folder FROM project WHERE folder = ?")
  const byId = db.query<Project, [string]>("SELECT id, name, folder FROM project WHERE id = ?")
  const insertProject = db.query("INSERT INTO project (id, name, folder, created) VALUES (?, ?, ?, ?)")
  const insertSession = db.query("INSERT INTO session (id, project, created) VALUES (?, ?, ?)")
  // `IS` compares with null as well as with an id. Only a session the owner
  // has said something in counts: one that holds nothing but a question
  // asked while it was opening is not a conversation to carry on.
  const lastUsed = db.query<{ id: string }, [string | null]>(
    `SELECT session.id FROM session JOIN event ON event.session = session.id
     WHERE session.project IS ?
       AND session.id IN (SELECT session FROM event WHERE type = 'user_message')
     GROUP BY session.id
     ORDER BY MAX(event.time) DESC, session.created DESC
     LIMIT 1`,
  )

  const findOrCreate = db.transaction((root: string): Project => {
    const existing = byFolder.get(root)
    if (existing) return existing
    const project: Project = { id: crypto.randomUUID(), name: basename(root), folder: root }
    insertProject.run(project.id, project.name, project.folder, Date.now())
    return project
  })

  return {
    forFolder(folder) {
      if (!existsSync(folder)) throw new Error(`The folder "${folder}" does not exist.`)
      if (!statSync(folder).isDirectory()) throw new Error(`"${folder}" is a file, not a folder.`)
      // Symbolic links are resolved so one folder is never two projects.
      const resolved = realpathSync(folder)
      return findOrCreate(projectRoot(resolved))
    },

    get(id) {
      return byId.get(id) ?? undefined
    },

    createSession(project) {
      const id = crypto.randomUUID()
      insertSession.run(id, project?.id ?? null, Date.now())
      return id
    },

    lastSession(project) {
      return lastUsed.get(project?.id ?? null)?.id
    },
  }
}
