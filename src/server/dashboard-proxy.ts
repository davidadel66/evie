// Passes a request of a live dashboard's page on to its app, on this
// computer, and its answer back. The page is reached at `<prefix>/…`, which
// stands for the app's own address, and runs in a sandbox: it has no origin
// of its own, so it can never read Evie's page or the owner's sign-in.

// Request headers kept from the app: those about this one connection, and
// the owner's sign-in. The browser's `accept-encoding` is dropped too, since
// Bun's fetch asks for compression itself and undoes it.
const NOT_PASSED_ON = new Set(["host", "connection", "keep-alive", "transfer-encoding", "upgrade", "te", "proxy-connection", "proxy-authorization", "authorization", "origin", "accept-encoding"])
// Answer headers kept from the browser. The body comes back already
// uncompressed, so its encoding and length no longer hold. A cookie the app
// sets would be Evie's own address's, where it could stand in for the
// owner's sign-in. The app's own framing and content rules give way to the sandbox.
const NOT_PASSED_BACK = new Set(["connection", "keep-alive", "transfer-encoding", "content-encoding", "content-length", "set-cookie", "x-frame-options", "content-security-policy"])
// The same as the iframe's sandbox, on every answer, so that a page opened
// on its own, as in a new browser tab, is sandboxed too.
const SANDBOX = "frame-ancestors 'self'; sandbox allow-scripts allow-forms allow-downloads allow-popups"

export async function forward(request: Request, upstream: string, prefix: string): Promise<Response> {
  const asked = new URL(request.url)
  const base = new URL(upstream)
  const basePath = base.pathname.replace(/\/+$/, "")
  const rest = asked.pathname.slice(prefix.length)

  // A sandboxed page has no origin, so a request it sends to the app that
  // is not a simple one is asked about first. The app is only ever reached
  // through its pass, so any page holding it may.
  if (request.method === "OPTIONS" && request.headers.has("access-control-request-method")) {
    return new Response(null, {
      status: 204,
      headers: {
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Methods": "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS",
        "Access-Control-Allow-Headers": request.headers.get("access-control-request-headers") ?? "*",
        "Access-Control-Max-Age": "600",
      },
    })
  }

  const headers = new Headers()
  request.headers.forEach((value, key) => {
    if (NOT_PASSED_ON.has(key)) return
    if (key === "cookie") {
      const kept = value
        .split(";")
        .map((part) => part.trim())
        .filter((part) => part !== "" && !part.startsWith("evie_token_"))
      if (kept.length > 0) headers.set("cookie", kept.join("; "))
      return
    }
    headers.append(key, value)
  })

  let answer: Response
  try {
    answer = await fetch(`${base.origin}${basePath}${rest}${asked.search}`, {
      method: request.method,
      headers,
      redirect: "manual",
      ...(request.method !== "GET" && request.method !== "HEAD" && request.body !== null && { body: request.body }),
    })
  } catch {
    return new Response("This dashboard's app is not answering. Reload the tab to start it again.", { status: 502 })
  }

  const back = new Headers()
  answer.headers.forEach((value, key) => {
    if (!NOT_PASSED_BACK.has(key)) back.append(key, value)
  })
  const location = back.get("location")
  if (location !== null) back.set("location", throughPass(location, base, basePath, prefix))
  back.set("Access-Control-Allow-Origin", "*")
  back.set("Cache-Control", "private, no-cache")
  back.set("X-Content-Type-Options", "nosniff")
  back.set("Content-Security-Policy", SANDBOX)
  return new Response(answer.body, { status: answer.status, statusText: answer.statusText, headers: back })
}

// Where a redirect of the app's goes, through the pass: an address of the
// app's own, under its base path, is moved under the prefix. One relative
// to the page, such as `login`, already is. Any other, such as another
// site's, is left as it is.
function throughPass(location: string, base: URL, basePath: string, prefix: string): string {
  if (!location.startsWith("/") && !/^[a-z][a-z0-9+.-]*:/i.test(location)) return location
  const to = URL.canParse(location, base.href) ? new URL(location, base) : undefined
  if (to === undefined || to.origin !== base.origin) return location
  if (to.pathname !== basePath && !to.pathname.startsWith(`${basePath}/`)) return location
  return `${prefix}${to.pathname.slice(basePath.length) || "/"}${to.search}${to.hash}`
}
