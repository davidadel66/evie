import type { Notice, NoticeMode } from "./notices.ts"

// The sound, made in the page, so that there is no file to fetch: two soft
// notes, rising when a session needs the owner and falling when a turn has
// ended. A browser plays sound only once the page has been used, so the
// player is made then, when the owner wants a sound.
let audio: AudioContext | undefined

// Makes the player, in a gesture of the owner's, such as a click, when
// the notifications are set to have a sound: as the page is used, and as
// the sound is chosen, so that the first notice after it is heard.
export function unlockSound(mode: NoticeMode) {
  if (mode !== "sound" && mode !== "both") return
  try {
    audio ??= new AudioContext()
    void audio.resume().catch(() => {})
  } catch {
    // No sound in this browser.
  }
}

// Plays the sound of a notice, once the player is made.
export function chime(notice: Notice) {
  if (audio === undefined || audio.state !== "running") return
  const notes = notice === "needs you" ? [660, 880] : [880, 660]
  notes.forEach((frequency, at) => {
    const start = audio!.currentTime + at * 0.14
    const tone = audio!.createOscillator()
    const loudness = audio!.createGain()
    tone.frequency.value = frequency
    loudness.gain.setValueAtTime(0.0001, start)
    loudness.gain.exponentialRampToValueAtTime(0.08, start + 0.02)
    loudness.gain.exponentialRampToValueAtTime(0.0001, start + 0.3)
    tone.connect(loudness).connect(audio!.destination)
    tone.start(start)
    tone.stop(start + 0.32)
  })
}
