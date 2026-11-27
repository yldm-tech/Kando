# Changelog

What changed in each version of Kando, newest first. The app shows the section of a new version when it offers the update.

## 0.76.0 - 2026-11-27

### Features

- Stream computer films over the local network (#524)
- Record AI usage per month and show it in Settings › AI (#526)
- Keep AI billing text and provider response bodies off the phone (#528)

### Fixes

- A work whose episodes were marked watched goes on with the next one (#525)

## 0.75.0 - 2026-11-27

### Features

- Show and unlock password-protected libraries on mobile (#522)

## 0.74.0 - 2026-11-27

### Features

- Load posters and Plex artwork for indexed films (#507)
- Fold a show's episodes into one row (with review fixes) (#521)

### Fixes

- Isolate apart keys and seed watchedSign on server start (#520)

## 0.73.0 - 2026-11-27

### Features

- Tell the phones which work each film is folded into (#518)

## 0.72.0 - 2026-11-27

### Features

- A show's new episode joins its categories, and the hero offers the next one (#517)

## 0.71.0 - 2026-11-27

### Features

- Fold a show's episodes into one card (#516)

## 0.70.0 - 2026-11-27

### Features

- Work out which films are one work, and keep a list's title when it is one (#515)

## 0.69.0 - 2026-11-26

### Features

- Read fansubs' episode names, and name bare-numbered films' folders after their show (#514)

## 0.68.0 - 2026-11-26

### Features

- Export the subtitles of every film of a library at once (#511)

### Fixes

- Leave the films a category hides out of the selection (#512)
- Play a show's folders in one order, its seasons by number (#513)

## 0.67.0 - 2026-11-26

### Features

- Keep a folder library's films' files in a hidden folder beside each video (#510)

## 0.66.2 - 2026-11-26

### Fixes

- Keep writing an outside video's files into the library while it is open (#508)

## 0.66.1 - 2026-11-26

### Fixes

- Read a link's lines by the name it goes by while its sound plays (#509)

## 0.66.0 - 2026-11-26

### Features

- Keep a video's own files in the library, not beside a video kept outside it (#506)

### Fixes

- Drop Mica on Windows until NSGo hides DWM's caption buttons (#504)

## 0.65.0 - 2026-11-26

### Features

- Follow a folder and import its new videos by themselves (#503)

### Fixes

- Require Kando 0.64.0 for kando.plex 1.1.0 (#502)

## 0.64.0 - 2026-11-26

### Features

- Choose a run of rows with Shift, filter the folder, ⌘A (#499)
- Sign in to Plex instead of copying a token from View XML (#495)
- Say how many videos of each folder are imported (#497)
- Import the remembered way without the dialog (#498)
- List the media sources in the sidebar (#500)

## 0.63.0 - 2026-11-26

### Features

- Choose all, and import a folder whole (#493)
- Install an official source plugin from the add dialog (#494)

## 0.62.0 - 2026-11-26

### Features

- Reach the media sources from the main window (#492)

## 0.61.0 - 2026-11-26

### Features

- Add the official Plex media-source plugin, kando.plex (#491)

## 0.60.0 - 2026-11-25

### Features

- List a NAS film matched to a computer's film once (#484)

### Fixes

- Stop a removed source's stream before forgetting the source (#487)
- Read Bilibili's HTTP 567 as a rate limit (#485)
- Keep a Jellyfin or Emby listing from hanging when tsx cuts it (#490)

## 0.59.0 - 2026-11-25

### Features

- Add the official Jellyfin media-source plugin, kando.jellyfin (#480)
- Add the official Emby media-source plugin, kando.emby (#483)
- Take a computer's subtitles for a NAS film by its source key (#482)

### Fixes

- Keep the step's button above the software keyboard on a phone (#481)

## 0.58.0 - 2026-11-25

### Features

- Add the official WebDAV media-source plugin, kando.webdav (#475)
- Stream a source's video through the loopback server (#476)
- Ask where the films are in the onboarding, and add a NAS there (#477)
- Add the official Alist media-source plugin, kando.alist (#478)

## 0.57.0 - 2026-11-25

### Features

- Browse a NAS or server from the phone and download its videos (#474)

## 0.56.0 - 2026-11-25

### Features

- Ask for iOS's local network access before pairing, on NSGo's new permission (#472)

## 0.55.1 - 2026-11-25

No changes to the app worth listing: this version only carries maintenance.

## 0.55.0 - 2026-11-25

### Features

- Pair a phone away from the computer's network through the storage relay (#470)

### Fixes

- Go on through the relay after one connection's time, not iOS's 30-second wait (#471)

## 0.54.0 - 2026-11-25

### Features

- Read a plugin's media sources through kando.source.fetch (#464)

### Fixes

- Wait out iOS's local network question and try the computer's addresses at once (#469)

## 0.53.0 - 2026-11-24

### Features

- Make an indexed film's lines from its source (#462)

## 0.52.0 - 2026-11-24

### Features

- Keep storage locations' passwords and keys in the system's secure store (#458)
- Add SFTP media sources, trusting the server's key at the first connection (#457)
- Let a script plugin declare media sources and kando.source.fetch (#456)
- Add WebDAV as a built-in kind of media source (#459)
- Index a media source's videos and play them from the source (#460)

### Fixes

- Keep a film's job going when its subtitles are being read on Windows (#455)
- Retry a move's rename on Windows while something holds the film (#453)

## 0.51.3 - 2026-11-24

### Fixes

- Build the phone app for Google Play's required target API again (#463)

## 0.51.2 - 2026-11-24

### Fixes

- Hear a voice the VAD is less sure of too, keeping only what a voice there could say (#451)

## 0.51.1 - 2026-11-24

### Fixes

- Keep a line said across the end of a run that the next run hears as part of the line before (#450)

## 0.51.0 - 2026-11-23

### Features

- Play a film the computer is still making, its lines made from where it is watched (#437)

## 0.50.0 - 2026-11-23

### Features

- Add whole folders, asking first when they hold many new videos (#444)
- List films being made to phones and make first the one a phone watches (#436)

## 0.49.0 - 2026-11-23

### Features

- Sort a library's films into categories (#440)
- Put the film you watch first, from redos to a link still downloading (#435)
- Log where a film is left untranslated, and why (#446)

### Fixes

- Keep the user's files and films when they move or their folder is gone (#438)
- Let the subtitle list and the picture's lines follow a drag of the scrubber (#439)
- List the speeds fastest at the top of the speed menu (#443)
- Stop a plugin whose JS keeps the window busy outside its calls (#445)

## 0.48.2 - 2026-11-22

### Fixes

- Read a pairing QR code that gozxing only reads turned (#433)

## 0.48.1 - 2026-11-22

### Fixes

- Keep the lines of a batch whose reply was cut off or written wrong, and go on past a missing batch that fails (#430)

## 0.48.0 - 2026-11-21

### Features

- Give the translator the earlier translations and the title, and keep translating through a cell or a service that fails (#423)

## 0.47.0 - 2026-11-21

### Features

- Translate the lines a gone service left once it answers again (#424)

### Fixes

- Keep a proxy URL served until every holder revoked it (#425)
- Copy a video at a share's root into a folder of its own (#426)
- Number a source's saves apart from its Tests (#427)
- Finish undoing the desktop workarounds of #409 (#428)
- Keep the secure store in step with settings.json when a save does not finish (#431)

## 0.46.0 - 2026-11-21

### Features

- Hear only the speech, and tell the language from it (#418)
- List the parts of a film being made that have no lines yet (#417)
- Show a film's new lines where a redo has made them (#415)
- Time each line by its words, and cut long lines where the speech pauses (#420)

### Fixes

- Keep the subtitle list's rows in place as lines come in before them (#414)

## 0.45.0 - 2026-11-21

### Features

- Add SMB media sources in Settings, with their passwords in the system's secure store (#412)
- Browse SMB sources and copy their videos into the library (#413)

### Fixes

- Give a detached image's disk its own time to eject, and run the vault tests one package at a time (#416)

## 0.44.0 - 2026-11-21

### Features

- Serve any Reader from the loopback proxy, check its Host and revoke a film's URL (#410)
- Add the registry of media source kinds, source addresses, Local and placeholder paths (#411)

## 0.43.2 - 2026-11-21

### Fixes

- Keep a file of the plugins folder saved while it is being read on Windows (#403)

## 0.43.1 - 2026-11-21

### Fixes

- Proofread a film heard in the language its subtitles are for (#387)

## 0.43.0 - 2026-11-21

### Features

- Scan the computer's pairing QR code with the camera's live picture (#391)

## 0.42.0 - 2026-11-21

### Features

- Draw the Android design's Material pieces (#363)

## 0.41.0 - 2026-11-21

### Features

- The player in the design's look (#370)

## 0.40.1 - 2026-11-21

### Fixes

- Hold a film's folder while its marks are written, so Windows can delete it (#393)

## 0.40.0 - 2026-11-21

### Features

- Design pass for onboarding and pairing (#364)
- Design look for Learn, the review, the word list and the export (#368)

## 0.39.0 - 2026-11-21

### Features

- Design look for the shell, and a Jobs tab in place of Browse (#352)
- Let a script plugin suggest glossary terms (kando.glossary.suggest) (#344)
- Design pass for the Library tab (#369)
- Pair by taking a photo of the computer's QR code (#366)
- Check packs against the revocation list in kando-plugin validate and index (#343)
- Show the version's release notes in the update dialogs (#388)
- Film page in the design's grouped look, and a row plays a film on the phone (#386)
- Say what a grant cannot limit, group permissions, log every decision (#345)
- Design look for the Jobs page and the Add sheet (#365)
- Word list frequency ranks on the lookup card (#346)
- Settings design pass (#367)

### Fixes

- Keep the manifest rules, examples and preview in step (#348)
- Host API gaps: plugins.list, one film id per film, install checks, missing-key precheck (#347)
- Keep showing what a panel's module mounts, keep the test tree (#355)
- Reading alignment and kana splits found reviewing the plugin screenshots (#349)
- Let a plugin's suggested term be dismissed when the suggestions file cannot be read (#371)
- Don't ask for a plugin's key when the secure store failed to read it (#372)
- Add a collection's or playlist's episodes when subscribing to it (#351)
- Say when a site's new page script waits, test the load it cuts short (#375)
- Keep the time watched until the film's library is known (#379)
- Finish a detach whose volume left after hdiutil gave up, retry a silent hdiutil create (#383)
- Answer a relayed marks ask from what the engine had at the ask (#381)
- Keep reading and cleaning a legacy secret setting (#373)
- Keywords that don't repeat the app's name, and tell App Review why the app asks for the camera (#390)

## 0.38.0 - 2026-11-20

### Features

- Take audio files, as mp3 and m4a, as films (#332)
- Ask for the local network in the phone's language (#336)
- Open kando:// links on Windows and Linux (#335)
- Take the chapters a local file marks in itself (#333)
- Play the next film of a course, disc, season or playlist, on the desktop and the phone (#334)

## 0.37.0 - 2026-11-20

### Features

- Plugin translation sources and the official DeepL plugin (#329)
- Plugin panels in the player's Extensions tab (#327)

### Fixes

- Require the Kando versions the official plugins need (#331)

## 0.36.0 - 2026-11-20

### Features

- The official AnkiConnect plugin, official signing and revocation publishing (#328)

## 0.35.0 - 2026-11-19

### Features

- Kando-plugin CI tool, kando:// install links, update notice and the /plugin page (#326)

## 0.34.0 - 2026-11-19

### Features

- Script plugin lookups, formats, exporters, annotators, AI budget and key proxy (#322)
- A signed community plugin directory and kando plugin index (#323)

## 0.33.0 - 2026-11-19

### Features

- Script plugins — sandboxed runtime, trust, permissions and commands (#314)
- Make a film's subtitles while it plays, from one subtitle document per film (#306)
- Learning tasks, translation styles, export templates and your own packs (#313)
- Word marks on phones, computed on the desktop (#318)

### Fixes

- Keep a moved video's path from another file, and make the remux note readable (#312)
- The review fixes for playhead-first making (#306) (#317)

## 0.32.0 - 2026-11-19

### Features

- Site packs for the built-in browser (#311)
- Move several videos at once, remux from a location, and the rest of #307's list (#310)

## 0.31.0 - 2026-11-19

### Features

- Keep films on NAS shares and object stores, and play them from there (#302)
- Underline levels, lookup hints, phone sources and safer state (#309)

## 0.30.0 - 2026-11-19

### Features

- Look words up in installed dictionaries, offline (#300)
- Word lists and readings over subtitles (#301)

## 0.29.1 - 2026-11-19

### Fixes

- Address the review of #298 and #299 (#304)

## 0.29.0 - 2026-11-19

### Features

- Plugin pack format, validation and the design (#298)

## 0.28.0 - 2026-11-19

### Features

- The glossary on phones, the Terms tab's edit log with undo, and suggested terms (#287)
- Learning state, settings, conflicts and subscriptions through the storage relay, and the storage's health on the phone (#283)

### Fixes

- Wait for a followed film's record before reading it (#297)
- Cut each line once for the Words tab count and coalesce the re-cut once a dictionary is read (#285)

## 0.27.0 - 2026-11-19

### Features

- Settle sync conflicts on the phone and send where a film was left to the computer (#265)
- Storage relay switch and health on the desktop, and phones following the queue through it away from home (#270)
- Notes tab, words in the review, audio kept at save, glossary term on lookups, Anki audio for words (#268)
- A reviewable final proofread, series from playlists and file names, and the subtitle progress badge (#271)
- Confirm sheet for online links, playback time from the extension and bookmarklet (#273)
- Job options, read-only phones, unfinished counts and a centred Add dialog on tablets (#266)
- Recap and Explain follow-ups on phone and tablet through the computer (#267)
- Words tab, Add to Vocabulary and the mini player in the learning player (#269)
- Study stats and the computers' subscriptions in the Library, over protocol v2 (#272)
- Dictation, export formats, a computer's settings, and every device's study time (#276)
- IOS widgets and hardware keyboard shortcuts (#275)
- Browse the video sites and send a page's video to the computer (#277)
- Store packaging for the phone app: icon, Android permissions, listings, privacy answers and a manual package workflow (#274)
- Word links for Japanese and Chinese, kept sounds follow their records, word ends over sync (#281)
- Series, a channel's latest videos, video details before adding, automatic captions as a reference layer, bilibili 480p (#280)
- Films through the user's storage: upload after export, stream or download away from home, storage use, cleanup and moving (#288)
- Subscriptions in the tablet sidebar, a film's computer actions, and films without a video on phones (#286)
- One prompt where a film's video would play, wherever the video is (#292)

### Fixes

- Gofmt the merged phone files, wait for the tab bar, and wait out a yt-dlp update before its folder goes (#282)
- Wait for the phone's tab bar to be mounted, and for Cancel to be enabled (#289)
- Drop captions a video lacks, and hide the reference line with the subtitles (#284)
- Two tests that failed on CI, and a sync that turned a computer's settings back on the phone (#291)
- Bring the two Chinese dictionaries compressed, 29 MB off the apps (#290)
- Subscribe to a bilibili upload of several parts from its video page (#293)
- Wait for Retry to take input, and for the widgets to count the streak (#294)
- The phone tests' texts read nothing while the view is not there yet (#296)

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
