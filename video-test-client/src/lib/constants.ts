// Mirrors the allowed values in ../../go-video/internal/httpapi/validate.go
// so the forms only offer inputs go-video will actually accept.

export const BG_VIDEOS = ["gtav", "minecraft", "roblox", "subways", "satisfying"] as const;

export const VOICES = [
  "en-US-BrianNeural",
  "en-US-AvaNeural",
  "en-US-AndrewNeural",
  "en-US-EmmaNeural",
  "en-US-JennyNeural",
  "es-BO-SofiaNeural",
  "es-BO-MarceloNeural",
  "es-MX-JorgeNeural",
  "es-MX-DaliaNeural",
  "es-DO-EmilioNeural",
] as const;

export const MUSICS = [
  "elevator",
  "else",
  "hiddenagenda",
  "nocturne",
  "sneakysnitch",
  "tiptoes",
  "wiener",
  "waltz",
] as const;

export type BgVideo = (typeof BG_VIDEOS)[number];
export type Voice = (typeof VOICES)[number];
export type Music = (typeof MUSICS)[number];
