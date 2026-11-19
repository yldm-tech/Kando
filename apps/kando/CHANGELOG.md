# Changelog

What changed in each version of Kando, newest first. The app shows the section of a new version when it offers the update.

## 0.26.0 - 2026-11-19

### Features

- Say when a program's copy does not start, run one that does, and fix it from where it shows (#261)
- Float the sidebar over the page in glass on macOS, as Finder does (#262)

## 0.25.0 - 2026-11-19

### Features

- Add up study stats across the unlocked libraries and name them (#247)
- Add at most three new videos per check, and subscriptions on the command line and MCP (#249)
- Candidates for a wrong line and fixes across the film; E selects a line and I explains it (#252)
- Storage relay spec, bucket reads and presigned links, and the desktop relay behind a setting (#250)
- Request size and retries, context lines, speech threads and hints, appearance and shortcut presets (#251)
- Show the whole hero card whenever the window has room for it (#248)
- Recap card when a film opens, and Explain Line in the side panel with follow-ups and save to notes (#254)
- Handoff, paused-on-phone jobs, read-only phones, conflicts and activity on the desktop (#256)
- One library of every paired computer's films, with continue watching, search over kept lines, source chips and a film's options (#253)
- Jobs page and Add sheet over protocol v2 (#257)
- Learning player with the line card, lookups and explanations through the computer, saved lines and notes over v2 (#258)
- Learn tab with review, vocabulary, saved lines and export; Settings for subtitle style, goal, reminder and a revoked computer (#259)
- Subtitle-only films that stream from YouTube on macOS and download their video later (#255)

## 0.24.0 - 2026-11-18

### Features

- New shell with Library, Learn and Settings tabs, a tablet sidebar and an introduction (#240)
- Export a film's subtitles as SRT, WebVTT or ASS, with the user's corrections (#238)
- Windows title bar and Linux header bar, with Ctrl+K reaching every menu command (#242)
- Look up words on the picture, vocabulary, time watched, study stats and vocabulary export (#244)
- Cut films at the chapters their site marks, with translated titles and a toggle back to even cuts (#243)
- Film, series and global terms the translation follows, a Terms tab, and series from subscriptions (#241)
- Learning state over protocol v2, merged field by field with tombstones and conflicts (#239)

### Fixes

- Keep a film's record when it is saved while being read or saved twice at once (#237)

## 0.23.0 - 2026-11-18

### Features

- Encrypt a locked library's files on macOS (#199)
- Mark films watched or unwatched, and filter the library by how far they were watched (#198)
- Select several films and act on them at once (#200)
- Correct lines, kept apart from the machine's subtitles, with undo (#234)
- Design tokens, platform fonts and radii, and the sidebar's cards (#231)
- Offline mode, with what each step sends over the network and a cache to clear (#233)
- Follow channels, collections and playlists on the desktop (#235)
- Protocol v2 with capabilities, jobs and events; phones send links to the desktop's queue (#230)
- Keep saved lines in a shared study record with notes, built for sync (#228)
- Take the site's own subtitles as the original lines, and choose the download quality (#232)
- Status icon, keep running with the window closed, launch at login and pausing the queue (#229)

### Fixes

- Seed a new vault image so it detaches in seconds, and lighten its tests (#201)
- Make vault images as before, and test them in a CI job of their own (#202)
- Try a busy vault image again before giving up on detaching it (#203)

## 0.22.0 - 2026-11-18

### Features

- Show the films as a list, and sort them either way or by length (#194)

### Fixes

- Show the whole picture instead of cropping it to fill the window (#189)
- A downloaded film takes its desktop's new subtitles when it is played (#191)
- Proofread what speech recognition misheard, and leave films in their own language untranslated (#190)
- Proofread Thai, Lao, Khmer and Myanmar a character at a time (#195)
- Move a film's hidden files to the Trash with it (#196)
- Let the list's Today and Yesterday follow the day (#197)

## 0.21.1 - 2026-11-18

No changes to the app worth listing: this version only carries maintenance.

## 0.21.0 - 2026-11-17

### Features

- Redesign the phone app with Downloads and Computers tabs and phone-sized controls (#187)

## 0.20.0 - 2026-11-17

### Features

- Sign Browse's pages in with the chosen browser's sign-in (#184)
- The iOS and Android app pairs, downloads and plays films (#185)

## 0.19.0 - 2026-11-17

### Features

- Move a film to another library (#180)
- Password-protect a library as an app lock (#181)

## 0.18.0 - 2026-11-17

### Features

- Serve the libraries to paired phones on the local network (#182)

## 0.17.0 - 2026-11-16

### Features

- Keep the subtitle position per film, and make moving it easy to find (#175)

## 0.16.2 - 2026-11-16

### Fixes

- Tell a browser folder macOS keeps from Kando from a missing one (#173)

## 0.16.1 - 2026-11-16

### Fixes

- Explain X posts whose sensitive or protected video X hides, and lead to the sign-in (#171)

## 0.16.0 - 2026-11-16

### Features

- Download with the sign-in of the user's own browser (#168)

## 0.15.0 - 2026-11-16

### Features

- Put Download Video and + buttons into the pages of video sites (#154)
- Browser extension that adds videos to Kando through native messaging (#160)

### Fixes

- Upload to http endpoints, without a 30-minute cap, and say why an upload failed (#150)
- Study tools act on the line just heard, export whole notes for Anki, and answer in the film's language (#151)
- Stop jobs overwriting subtitles and misreporting a failed Process Again (#152)
- Library and sidebar filters, focus and screen reader labels (#157)
- Tell the CLI and MCP how a job really ended, and let them add folders and open films being processed (#156)
- Windows and Linux platform gaps (console windows, tool sources, poster URL, AppImage CLI, GStreamer plug-ins, paths) (#153)
- AI presets with chat models, per-service keys, custom endpoints in any order, and a one-click hf-mirror.com download (#155)
- Close the dark-mode gaps in Settings › AI, saved lines, sidebar, library and player (#159)
- Translate original-only films in bulk and stop describing translations they don't have (#161)
- Platform-neutral wording, Explain Line strings, Chinese language names, playlists in the guide, save errors and dead keys (#164)

## 0.14.1 - 2026-11-16

### Fixes

- Drop the cards inside the sidebar, so the switcher and the status are rows of it (#149)

## 0.14.0 - 2026-11-16

### Features

- Type or search AI models, choose a whisper model file, more languages (#135)
- Open sites from Browse, say what Download Video did, pause hidden videos (#139)
- Search subtitle lines, read part of a film, open at a line, and make scripting safe (#134)
- Set up translation in the first-run guide and trim the AI and speech pages (#137)
- Study tools: pause after each line, multi-line loops, a list that follows the mode and lets the user read away (#140)
- Make the library, player and palette reachable by keyboard and readable by screen readers (#136)
- Add a playlist's or a channel's videos one job each, from the library, Browse, kando add and MCP (#144)
- Check speech model downloads against pinned checksums and offer hf-mirror.com (#143)
- Lead with the film being watched, show progress and spoken language on posters (#148)
- Explain Line, remedies under dead ends, subtitle style in Settings, and a selectable held line (#145)
- Keep films watchable through failures, show jobs everywhere, and pick the translation language per film (#138)
- Filter by spoken language in the sidebar and correct it per film (#146)
- Save lines, list them, export to Anki with audio, and review them (#147)
- Follow the system's light or dark appearance (#133)

### Fixes

- Open a film where it stopped from its poster, not at 0:00 (#125)
- Choose the new AI service's model, keep Test Connection current, stop on out-of-credit errors (#126)
- Make the palette's highlight match Return, add a palette button without a menu bar, and share shortcut labels and search (#128)
- Upload without a Google account, under kando/ when signed out (#130)
- Refuse pages that list videos, keep message bars current, search from Browse (#132)
- Truthful mode and clip notes, line-aligned clips, wider cue clock, finer speeds (#129)
- Keep a film watchable when Process Again fails, resume stopped translations, and say why Retry is refused (#127)
- Make subtitles of folder videos, keep them on failure, and add dropped folders (#142)

## 0.13.3 - 2026-11-16

### Fixes

- Use NSGo's windows and one instance on Windows and Linux, and check pixels on a platform (#122)

## 0.13.2 - 2026-11-15

No changes to the app worth listing: this version only carries maintenance.

## 0.13.1 - 2026-11-15

### Fixes

- Let Kando 0.12.0 and older update on macOS and Windows: this update replaces the MyGo build with the NSGo build of 0.13.0 and later, in the same place and with your libraries and settings, and later versions arrive through the new updater. On Linux the NSGo build needs GTK 4 and GStreamer, so it is not installed over the old one: remove the old copy with `curl -fsSL https://github.com/yldm-tech/Kando/releases/download/v0.12.0/install.sh | sh -s -- --uninstall` (your libraries and settings stay), then install the Debian or Arch package with `curl -fsSL https://github.com/yldm-tech/Kando/releases/latest/download/install.sh | sh`, or the AppImage on other distributions

## 0.13.0 - 2026-11-15

### Features

- Move Kando from MyGo to NSGo (#117)

## 0.12.1 - 2026-11-14

### Fixes

- Build on MyGo v0.9.0 and nodra v1.5.0, and quit without asking at logout or shutdown (#114)

## 0.12.0 - 2026-11-11

### Fixes

- Keep a tool kit's archive entries inside its folder on Windows too

## 0.11.2 - 2026-11-11

### Fixes

- Make the kando command, MCP redial and socket tests work on Windows
- Show the whole guide on Windows and fix Unix-only test assumptions
- Follow a renamed library on Windows
- Update a yt-dlp that is running

## 0.11.1 - 2026-11-11

No changes to the app worth listing: this version only carries maintenance.

## 0.11.0 - 2026-11-11

No changes to the app worth listing: this version only carries maintenance.

## 0.10.0 - 2026-11-10

No changes to the app worth listing: this version only carries maintenance.

## 0.9.2 - 2026-11-10

### Fixes

- Start a tool again while Linux says its file is busy

## 0.9.1 - 2026-11-10

### Fixes

- Keep a stale yt-dlp read from overwriting a newer one

## 0.9.0 - 2026-11-10

No changes to the app worth listing: this version only carries maintenance.

## 0.8.1 - 2026-11-10

### Fixes

- Say a video has no sound instead of showing ffmpeg's error

## 0.8.0 - 2026-11-10

### Features

- Drag subtitles up and down, off a film's burned-in lines

## 0.7.0 - 2026-11-10

### Fixes

- Queue a link typed without https:// instead of refusing it

## 0.6.0 - 2026-11-10

### Features

- Play sound on macOS, Linux and Windows through mygo/audio
- Add a built-in browser whose sign-ins carry into downloads

### Fixes

- Keep job records when another library is offline or library.json is damaged
- Stop the running job on quit, confirm quitting mid-job, keep Kando running when the main window closes on macOS
- Save the resume position while playing and remember subtitle delay per film
- Explain the everyapi CLI EveryAPI needs, and find it again without restarting
- Resume whisper model downloads and detect stalled downloads

## 0.5.1 - 2026-11-10

### Fixes

- Reject channel and list links, and do not queue the same video twice
- Use an existing .bilingual.srt beside a video that has no job record
- Esc leaves full screen first, and double-clicking the picture toggles full screen

## 0.5.0 - 2026-11-10

### Features

- Make the icon an amber glass speech bubble with a play mark
- Show updates in Settings › General

## 0.4.0 - 2026-11-09

### Features

- Check for updates from the app menu

## 0.3.0 - 2026-11-09

### Features

- Replace the app icon with a play mark over a pair of subtitle lines

## 0.2.0 - 2026-11-09

### Features

- Download yt-dlp, ffmpeg and whisper-cli on first launch
- Kando command line, MCP server and WebMCP

## 0.1.0 - 2026-11-09

### Breaking changes

- Switch config dir, library extension and env vars to Kando

### Features

- Scaffold native lingocut monorepo
- Add yt-dlp media pipeline boundary
- Connect native UI to video downloader
- Run download, transcription, translation and export pipeline
- Translate through EveryAPI by default with retries
- Keep processed videos in a sidebar library
- Restyle the library as a dark sidebar and film cards
- Import local videos with native file picker
- Implement audio playback for macOS and stub for other platforms
- Enhance library management and playback features
- Integrate nodra for session management and enhance library functionality
- Enhance icon handling and library logo functionality
- Enhance library renaming functionality and inode management
- Implement componentized UI structure using Onevra
- Integrate Google OAuth and Cloudflare R2 support
- Add AI logo integration and provider logo functionality
- Add Amazon S3 support alongside Cloudflare R2
- Add Alibaba Cloud OSS support for video uploads
- Add Tencent Cloud COS and S3-compatible storage support
- Phase 1 foundation: light theme, sub-states, shortcut table and page contracts
- Serial job queue, restart resume and library-switch stop (P1-2)
- Sidebar as a floating card with sources, library switcher and status line
- Library page in parts, drop-to-queue and remove to Trash (P1-3)
- Full-bleed player (P1-4)
- Settings window with categories, search and password fields (P1-5)
- Native menu bar and glass shortcut cheat sheet (P1-6)
- Add the feedback dialog that opens a prefilled GitHub issue
- Show real in-stage progress and the video's title on processing cards
- Classify yt-dlp failures and update yt-dlp the way it was installed
- Offer the remedy for each kind of failed download
- Add the ⌘K command palette
- First-run guide that opens 我的影片.kando, sets languages and downloads a speech model
- Pick the AI model from the service's own list, after the API key
- Animate side panel transitions
- Polish player panel and clip previews
- Add volume control and shortcuts
- List the sites yt-dlp downloads from, with their logos
- Use the video's own cover instead of a grabbed frame
- Interface in five languages, and each README in its own language

### Fixes

- Translate with the signed-in EveryAPI account
- Handle encoded local video paths
- Close the open film on a click of the current sidebar item and keep Mica on Windows
- Register the locales catalog so MyGo texts follow the bundle languages
- Center the upload capsule in the room the side panel leaves
- Keep the EveryAPI key off other LLM providers
- Stop the film's sound when the app ends
- Say 生成字幕 instead of 转录 for the speech stage
- Keep the source counts readable on the focused selection
- Tell yt-dlp where ffmpeg is so a Finder-launched app saves the sound
- Find browsers by the login folders yt-dlp reads, and offer Safari only on macOS
- Read yt-dlp's account-credentials login hint as a login, and an unresolvable host as its own kind
- Read browsers off the UI thread, offer no Safari fallback, and make 换个链接 match the prototype
- Scrub every storage credential and whole paths from the issue
- Keep yt-dlp partial files outside the library
- Retry failed batches, keep the film when some lines fail, say why in Chinese
- Keep the resume point when a film is only opened
- Follow a library renamed while the app was closed
- Picture beside the side panel, a top-right panel toggle, and room around the back button
- Restore missing translation language
- Recover short translation batches
- Separate back control from traffic lights
- Keep audio aligned after seeking
- Keep background playback smooth
- Keep picture and sound level when coming back to the app
- Join the sound once the window shows again

### Performance

- Speed up Whisper transcription
