/** When something happened, relative to now for the last day: "Just now", "12m ago", "3h ago", then "Oct 5". */
export function ago(at: string, now: Date): string {
  const then = new Date(at);
  const minutes = Math.floor((now.getTime() - then.getTime()) / 60_000);
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes}m ago`;
  if (minutes < 24 * 60) return `${Math.floor(minutes / 60)}h ago`;
  return then.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

/** A length of time at a glance: "41s", "2m 06s", "4m", "3h 12m". */
export function duration(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 10) return seconds % 60 === 0 ? `${minutes}m` : `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  return minutes % 60 === 0 ? `${hours}h` : `${hours}h ${minutes % 60}m`;
}
