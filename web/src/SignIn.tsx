import { useState, type FormEvent } from "react"
import { ConnectionLight } from "./ConnectionLight.tsx"
import type { Connection, ConnectionState } from "./connection.ts"

// The page when the browser is not signed in. On evie's own computer,
// `evie web` opens the page signed in; on another device, a token is pasted.
export function SignIn({ connection, state }: { connection: Connection; state: Extract<ConnectionState, { status: "signed out" }> }) {
  const [token, setToken] = useState("")
  const [failed, setFailed] = useState<string>()
  const [signingIn, setSigningIn] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (token.trim() === "") return setFailed("Paste a token first.")
    setSigningIn(true)
    setFailed(await connection.signIn({ token: token.trim() }))
    setSigningIn(false)
  }

  const said = failed ?? state.reason
  return (
    <main className="flex min-h-dvh justify-center bg-bg px-7 py-8 wide:items-center">
      <div className="flex w-full max-w-[400px] flex-col gap-6">
        <div className="flex items-center gap-3">
          <span className="text-title text-text">evie</span>
          <ConnectionLight state={state} />
        </div>

        <div className="flex flex-col gap-3">
          <h1 className="text-page-title text-text">Sign in</h1>
          <p className="text-body text-text-muted">
            On the computer evie runs on, run <code className="font-mono text-code text-text">evie web</code>. It opens this page, signed in.
          </p>
          <p className="text-body text-text-muted">
            On another device, paste a token made for it on that computer with{" "}
            <code className="font-mono text-code text-text">evie token add &lt;name&gt;</code>.
          </p>
        </div>

        <form onSubmit={submit} className="flex flex-col gap-3">
          <label htmlFor="token" className="text-label text-text">
            Token
          </label>
          <input
            id="token"
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={token}
            onChange={(event) => setToken(event.target.value)}
            className="h-touch-min rounded-md border border-line bg-surface px-3 font-mono text-code text-text"
          />
          {said !== undefined && (
            <p role="alert" className="text-body text-text">
              <Words text={said} />
            </p>
          )}
          <button type="submit" className="h-touch-min rounded-md bg-text px-4 text-label text-bg">
            {signingIn ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </div>
    </main>
  )
}

// A message in words for the owner, with the commands in it, written
// between backticks, shown as code.
function Words({ text }: { text: string }) {
  return text.split("`").map((part, at) =>
    at % 2 === 1 ? (
      <code key={at} className="font-mono text-code">
        {part}
      </code>
    ) : (
      part
    ),
  )
}
