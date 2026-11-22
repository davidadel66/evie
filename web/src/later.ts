import { createElement, lazy, Suspense, useEffect, useSyncExternalStore, type ComponentType, type ReactNode } from "react"

// What the page loads later: a part opened rarely or later, such as the
// settings, is imported with import(), so that the bundler builds it into a
// file of its own and the page's first script does not carry it. The page
// fetches it once it is wanted, or once the page is idle, and from then on
// draws it at once, so that the first opening does not wait. React's `lazy`
// and `Suspense` draw it, as T3 Code and OpenCode load their heavier panels.

export type Later<Module> = {
  // The module, fetched once however often it is asked for; one that could
  // not be fetched, as when the network is down, is fetched again on the
  // next ask.
  load(): Promise<Module>
  // Starts fetching it, for a caller that does not wait for it.
  preload(): void
  // The module once it is here, and undefined until then.
  loaded(): Module | undefined
  // Tells `listener` once it arrives.
  subscribe(listener: () => void): () => void
}

export function later<Module>(importer: () => Promise<Module>): Later<Module> {
  let module: Module | undefined
  let loading: Promise<Module> | undefined
  const listeners = new Set<() => void>()
  const load = () =>
    (loading ??= importer().then(
      (arrived) => {
        module = arrived
        for (const listener of [...listeners]) listener()
        return arrived
      },
      (cause: unknown) => {
        loading = undefined
        throw cause
      },
    ))
  return {
    load,
    preload: () => void load().catch(() => {}),
    loaded: () => module,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
  }
}

// A component of a module loaded later, which React's `lazy` draws: until
// the module is here, the nearest Suspense's fallback. One whose module is
// already here, as once the page has been idle, is drawn in the same
// render, with no frame of the fallback: `lazy` takes it from a thenable
// that calls back at once, as it would from a promise already settled, and
// a promise never calls back at once.
export function lazyFrom<Module, Props>(module: Later<Module>, pick: (loaded: Module) => ComponentType<Props>) {
  return lazy(() => {
    const here = module.loaded()
    if (here === undefined) return module.load().then((loaded) => ({ default: pick(loaded) }))
    const ready = { default: pick(here) }
    return { then: (resolve: (value: typeof ready) => void) => resolve(ready) } as unknown as Promise<typeof ready>
  })
}

// Whether a module loaded later is here, redrawn once it arrives. (The
// same answer is given to React's server rendering, which the tests use.)
export function useLoaded(module: Later<unknown>): boolean {
  const here = () => module.loaded() !== undefined
  return useSyncExternalStore(module.subscribe, here, here)
}

// A screen of a module loaded later, on the page once its module is here,
// and kept there from then on, so that it closes as it always has and keeps
// what it holds between openings. Wanted before then, the module is fetched
// for it, and `fallback` shows until it is here. The screen is drawn only
// once its module is here, so that one that could not be fetched, as while
// offline, leaves the page as it is, rather than failing it: the next time
// the screen is drawn, it is fetched again.
export function Screen({ loads, wanted, fallback = null, children }: { loads: Later<unknown>; wanted: boolean; fallback?: ReactNode; children: ReactNode }) {
  const here = useLoaded(loads)
  useEffect(() => {
    if (wanted && !here) loads.preload()
  })
  if (!here) return wanted ? fallback : null
  return createElement(Suspense, { fallback }, children)
}

// Runs `run` once the page is idle, after what it is doing now, such as
// drawing the session list: with requestIdleCallback where the browser has
// it, and a short timer where it does not, as in Safari. Gives back what
// calls it off.
export function whenIdle(run: () => void): () => void {
  const page = globalThis as { requestIdleCallback?: (run: () => void, options: { timeout: number }) => number; cancelIdleCallback?: (asked: number) => void }
  if (page.requestIdleCallback !== undefined && page.cancelIdleCallback !== undefined) {
    const asked = page.requestIdleCallback(run, { timeout: IDLE_AT_MOST })
    return () => page.cancelIdleCallback?.(asked)
  }
  const timer = setTimeout(run, IDLE_TIMER)
  return () => clearTimeout(timer)
}
// How long the page waits for an idle moment at most, and the timer where
// there is no idle callback.
const IDLE_AT_MOST = 2000
const IDLE_TIMER = 300
