// The gap between "sent" and the first token. With a reasoning model that gap
// is where the thinking happens and can run for many seconds — silence there
// reads as a hang. A stopgap until reasoning deltas stream for real, at which
// point this is replaced by the actual thought content.

export function Waiting() {
  return (
    <div className="flex flex-none items-center gap-[9px] self-start">
      <span className="relative flex h-[7px] w-[7px] items-center justify-center">
        <span
          className="border-amber absolute h-[7px] w-[7px] rounded-full border"
          style={{
            animation: "evepulse 1.6s ease-out infinite",
            transformOrigin: "center",
          }}
        />
        <span className="bg-amber h-[5px] w-[5px] rounded-full" />
      </span>
      <span className="text-muted-text font-sans text-[11.5px]">
        Evie is thinking…
      </span>
    </div>
  );
}
