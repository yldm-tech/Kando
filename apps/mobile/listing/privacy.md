# Privacy answers for the stores

What the phone app (apps/mobile) sends anywhere, and the answers it gives to the App Store's App Privacy questions and Google Play's Data safety form. Check them again whenever the app starts to reach a new host, links a new SDK or asks for a new permission.

## What leaves the device

- To the computers the user paired the phone with, and nothing else: the libraries, films, subtitles, playback positions, saved words and lines, notes, reviews and links the user sends there (session/, packages/sync). The connection is HTTPS over the local network, pinned to the computer's certificate fingerprint taken at pairing (packages/sync), so only that computer can read it. A film the computer is still making plays from a server of the app's own on the phone's loopback address, which the system's player reads inside the phone and which forwards each request to the computer over that pinned HTTPS (loopback/); it listens on 127.0.0.1 alone, so nothing off the phone can reach it. Kando's developers run no server the app talks to.
- Away from the computer's network, when the user turned on the storage relay on the computer: the same calls and replies, sealed with a key the computer hands the phone over the local network only, through the user's own S3-compatible bucket set up on the computer, by links the computer presigns for it (session/relay.go, packages/sync/relay.go). The phone holds no key of the bucket, and the bucket is the user's, not the developer's.
- Look-ups, Explain, questions about a film and recaps go to the paired computer, which sends the lines they are about to the AI service the user set up in Kando there; the phone itself reaches no AI service. That service is EveryAPI unless the user chose another, and EveryAPI is run by YLDM, Kando's developer, which receives those lines under the user's EveryAPI account and passes them to the model provider that answers.
- The only internet address the app opens is the desktop app's page on GitHub (onboarding.DownloadURL), in the system's browser. The app has no in-app browser.
- No account in the phone app, no analytics, no crash reporting, no advertising and no third-party SDK: the app imports only NSGo and Kando's own packages.

## Privacy policy

The public privacy policy is the site's privacy page, https://kando.yldm.ai/privacy/ in English and /zh-CN/privacy/, /zh-TW/privacy/, /ja/privacy/ and /ko/privacy/ in the other languages (apps/site, the privacy… strings of its content files). Each App Store listing gives its language's page in privacy_url.txt; Google Play takes the policy's address in Play Console's App content › Privacy policy, which fastlane's supply does not upload, so enter https://kando.yldm.ai/privacy/ there. Change the page whenever this file's answers change.

## App Store: App Privacy

Other User Content, Linked to You, for App Functionality alone; not used to track. Apple counts data as collected when it is sent off the device where the developer or a third party can reach it. The app sends data only to the user's own paired computers, but the lines a look-up, Explain, a question or a recap started on the phone are about reach EveryAPI, run by the developer, when the computer uses it, which keeps them with the user's EveryAPI account; so the answer declares them rather than Data Not Collected, whichever service the user picked. appstore/app_privacy_details.json holds this answer in the format fastlane's upload_app_privacy_details_to_app_store reads; it was published to App Store Connect with `asc web privacy apply` and `asc web privacy publish` (app 6836794833), and `asc web privacy pull` shows what is live.

Permissions the app asks for, in apps/mobile/nsgo.yaml: Local Network (NSLocalNetworkUsageDescription), to reach the paired computer; notifications, only when the user turns on the review reminder in Settings, which is scheduled on the device.

Required-reason APIs, which nsgo build declares in the app's privacy manifest (PrivacyInfo.xcprivacy): NSGo's own, user defaults (CA92.1), file timestamps (C617.1) and the system's uptime (35F9.1), and disk space (85F4.1, apple.privacy in nsgo.yaml), which Settings › Offline Films reads to show the user the films' share of the device and the room left. The reading stays on the device, so App Privacy's answer does not change.

## Google Play: Data safety

The same answer as the App Store's, entered in Play Console's App content › Data safety, which fastlane's supply does not upload:

- Does your app collect or share any of the required user data types? Yes: App activity › Other user-generated content, collected, not shared (EveryAPI is run by the developer, and the model providers it passes lines to process them on its behalf, which Google Play does not count as sharing), processed not ephemerally, required for the features that use it (Look-ups, Explain, questions and recaps), for App functionality alone. Everything else goes only from the phone to the user's own paired computer, over a connection encrypted end to end between the two (HTTPS pinned to the computer's certificate, or, through the storage relay, messages sealed with a key only the two hold, in the user's own bucket), which Google Play does not count as collection.
- Is all of the user data collected by your app encrypted in transit? Yes: phone to computer as above, and computer to the AI service over HTTPS.
- Do you provide a way for users to request that their data be deleted? Yes: through EveryAPI's account deletion. The user deletes films from the phone in the app, and unpairing a computer stops all transfers.

Permissions in the Android manifest: INTERNET and ACCESS_NETWORK_STATE (NSGo's runtime), to reach the computer; RECEIVE_BOOT_COMPLETED (NSGo's runtime), which schedules the review reminder again after a reboot; POST_NOTIFICATIONS (nsgo.yaml), for the review reminder.

