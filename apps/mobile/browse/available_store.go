//go:build store

package browse

// Available reports whether the app has Browse. Store builds leave it out (the store build tag), as App Store guideline 5.2.3 and Google Play's policy on downloading from other services may refuse an app that hands their videos to a downloader; other builds have it.
const Available = false
