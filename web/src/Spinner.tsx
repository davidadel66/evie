// The small spinner beside the word Working, or Connecting. It stands
// still for someone who has asked for less motion.
export function Spinner() {
  return <span aria-hidden="true" className="size-icon-xs shrink-0 animate-spin rounded-full border-(length:--indicator-width) border-current border-t-transparent motion-reduce:animate-none" />
}
