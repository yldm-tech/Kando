# Changelog

What changed in each version of Kando, newest first. The app shows the section of a new version when it offers the update.

## 0.5.0 - 2026-11-10

### Features

- Show updates in Settings › General
- Make the icon an amber glass speech bubble with a play mark

## 0.4.0 - 2026-11-09

### Features

- Check for updates from the app menu

## 0.3.0 - 2026-11-09

### Features

- Redesign the landing page around the player's bilingual subtitles
- Replace the app icon with a play mark over a pair of subtitle lines
- Trade the violet glows for the new icon's blue and subtitle yellow

### Fixes

- Say that macOS and Linux builds are out instead of that no version is

## 0.2.0 - 2026-11-09

### Features

- Download yt-dlp, ffmpeg and whisper-cli on first launch
- Let other programs drive the running app over a local socket
- Add the kando command line
- Serve Kando's library and queue to AI agents with kando mcp
- Register WebMCP tools on the landing page
- Add a command line and MCP card to Settings › General

## 0.1.0 - 2026-11-09

### Breaking changes

- Switch config dir, library extension and env vars to Kando

### Features

- Scaffold native lingocut monorepo
- Add yt-dlp media pipeline boundary
- Run download, transcription, translation and export pipeline
- Translate through EveryAPI by default with retries
- Connect native UI to video downloader
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
- Keep the window on screen across mygo dev reloads
- ParseLink reads the platform, video, list or channel of a link
- Light theme, sub-states, shortcut table and page contracts for phase 1
- Add Checked to the shortcut table and a modal flag the dialogs hold
- Close the open film in the player's shelfChanged once it has left the shelf
- Skip the download stage for local videos and stamp CompletedAt
- Requeue interrupted jobs on load and keep resume positions
- Run jobs in a serial queue
- Full-bleed player with transport, scrubber, subtitle layer, mode menu and side panel
- Draw the cheat sheet as dark glass with keycaps in columns
- Install a native menu bar built from the shortcut table
- Sidebar as a floating card with sources, library switcher and status line
- Open the settings in a window of their own
- Export VideoDuration for the library's length chips
- Split the library page into its parts and queue what is dropped
- Lay the cheat sheet out as the prototype's, near the window's top over a scrim
- Draw the settings window as the prototype's sheet
- Match the player to the prototype's glass, title bar, transport and side panel
- Add the first-run guide that sets languages, downloads a speech model and opens 我的影片.kando
- Report each stage's progress and the video's title through a trace
- Show the real stage progress, title and thumbnail on processing cards
- Add the feedback dialog that opens a prefilled GitHub issue
- Send one question to the translation model with ChatTranslator.Chat
- Keep how far each video was ever played in library.json
- Add the ⌘K command palette with subtitle search, commands and questions about the film
- Classify yt-dlp failures and update yt-dlp the way it was installed
- Offer the remedy for each kind of failed download
- Fold the side panel away from a button at the title bar's top right
- Pick the AI model from the service's own list, after the API key
- Animate side panel transitions
- Polish player panel and clip previews
- Add volume control and shortcuts
- List the sites yt-dlp downloads from, with their logos
- Use the video's own cover instead of a grabbed frame
- Show the interface in five languages
- Render the landing page from apps/site
- Simplified Chinese copy
- Korean copy
- Traditional Chinese (Taiwan) copy
- Japanese copy

### Fixes

- Translate with the signed-in EveryAPI account
- Handle encoded local video paths
- Always wait for the new window before stopping the old build
- Close the open film on a click of the current sidebar item and keep Mica on Windows
- Check a shortcut's When again when it runs and close the cheat sheet on ⌘/
- Give the subtitle line order one owner and stop saving the default look
- Save settings.json after the areas restore theirs when startup follows a renamed library
- Close the open film from the events that take it away, not from the page
- Keep mygo's full screen and Edit extras in the menu bar and hold its commands behind a file panel
- Write Escape as Esc on the cheat sheet's keycap
- Keep the content's drag band at the sidebar card's top row on macOS
- Ask for the EveryAPI sign-in once at a time and take a passing 测试 as an answer
- Drop library counts from a run that ends after a later one
- Leave the room under the title bar to a page that draws its own header bar
- Find the whisper models on disk outside the 语音识别 page
- Point the upload pill to 设置 › 账号与上传
- Show the library after a drop, ask for yt-dlp for local videos, keep nested folders out of the Trash
- Stop the shelf's scroll view from cutting the hero's shadow into a gray band
- Say the library page draws its own header bar under the title bar
- Draw the library's header bar across the content column from its top
- Show the side panel without a picture and start the in-column header bar at the column's edge
- Fill the player's window with the picture instead of letterboxing it
- Match the sidebar card to the prototype's boxes, rings and type
- Lay the library page out as the prototype does
- Place the player's 30-second and subtitle keys where the prototype's cheat sheet puts them
- Register the locales catalog so MyGo texts follow the bundle languages
- Center the upload capsule in the room the side panel leaves
- Keep the EveryAPI key off other LLM providers
- Stop the film's sound when the app ends
- Bump github.com/yldm-tech/ai-logo/mygo
- Say 生成字幕 instead of 转录 for the speech stage
- Keep the source counts readable on the focused selection
- Keep the issue link under the length GitHub's sign-in redirect accepts
- Say what translation needs without claiming a sign-in check that has not run
- Scrub every storage credential and whole paths from the issue
- Keep settings and libraries across a guide left open, a damaged settings.json and lost settings
- Count only playing on from what was watched, not seeks, as how far a film was played
- Key resume positions and furthest points by the new path when videos move on disk
- Tell yt-dlp where ffmpeg is so a Finder-launched app saves the sound
- Find browsers by the login folders yt-dlp reads, and offer Safari only on macOS
- Read yt-dlp's account-credentials login hint as a login, and an unresolvable host as its own kind
- Read browsers off the UI thread, offer no Safari fallback, and make 换个链接 match the prototype
- Keep the resume point when a film is only opened
- Follow a library renamed while the app was closed
- Retry failed batches, keep the film when some lines fail, say why in Chinese
- Keep yt-dlp partial files outside the library
- Keep a stopped card's own words and strip a killed tool's prefix
- Keep partial downloads on the library's volume
- Keep the picture beside the side panel instead of under it
- Leave room between the traffic lights and the back button
- Restore missing translation language
- Recover short translation batches
- Preserve context when splitting translation
- Separate back control from traffic lights
- Keep style reset action visible
- Keep audio aligned after seeking
- Pause playback while app is inactive
- Keep the frame visible while resyncing
- Only resync the picture when it fell behind the sound
- Address volume review findings
- Follow the sound behind a hidden window
- Join the sound once the window shows again
- Wrap commands, mark links and fix contrast and type
- Write site names in the page's script and language
- Never redirect a move to the English page within the site
- Say only what the app and the release do
- Give Japanese commands the same room as the others

### Performance

- Use more whisper threads
