// The small spinner beside the word Working, or Connecting. It stands
// still for someone who has asked for less motion.
export function Spinner() {
  return <span aria-hidden="true" className="size-3 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent motion-reduce:animate-none" />
}
