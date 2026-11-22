import { useEffect, useState } from "react"

// Phone width, as T3 Code's sidebar has it: 767px and narrower. Below it
// the session list is a sheet, the message box is one row until tapped,
// the settings window and the diff fill the screen, and a workspace's
// columns stack, and the session details are a sheet. The stylesheet's
// `wide` breakpoint is the same width.
export const WIDE = "(min-width: 768px)"

// Whether the page is wider than a phone, redrawn as it changes.
export function useWide() {
  const [wide, setWide] = useState(() => matchMedia(WIDE).matches)
  useEffect(() => {
    const query = matchMedia(WIDE)
    const change = () => setWide(query.matches)
    query.addEventListener("change", change)
    return () => query.removeEventListener("change", change)
  }, [])
  return wide
}
