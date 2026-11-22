// Asks again for what goes stale while the page is open, such as the branch
// each project is on, which changes with a checkout and is sent with no
// event: when the window has the focus back, and at each interval while
// the page is shown. Gives back the function that stops it.
export function refreshWhileShown(refresh: () => void, page: { window: EventTarget; shown(): boolean; every: number }): () => void {
  page.window.addEventListener("focus", refresh)
  const timer = setInterval(() => {
    if (page.shown()) refresh()
  }, page.every)
  return () => {
    page.window.removeEventListener("focus", refresh)
    clearInterval(timer)
  }
}
