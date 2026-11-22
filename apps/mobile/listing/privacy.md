# Privacy answers for the stores

What the phone app (apps/mobile) sends anywhere, and the answers it gives to the App Store's App Privacy questions and Google Play's Data safety form. Check them again whenever the app starts to reach a new host, links a new SDK or asks for a new permission.

## What leaves the device

- To the computers the user paired the phone with, and nothing else: the libraries, films, subtitles, playback positions, saved words and lines, notes, reviews and links the user sends there (session/, packages/sync). The connection is HTTPS over the local network, pinned to the computer's certificate fingerprint taken at pairing (packages/sync), so only that computer can read it. Kando's developers run no server the app talks to.
- Away from the computer's network, when the user turned on the storage relay on the computer: the same calls and replies, sealed with a key the computer hands the phone over the local network only, through the user's own S3-compatible bucket set up on the computer, by links the computer presigns for it (session/relay.go, packages/sync/relay.go). The phone holds no key of the bucket, and the bucket is the user's, not the developer's.
- Look-ups and Explain go to the paired computer, which asks the AI service the user set up in Kando there; the phone itself reaches no AI service.
- The only internet address the app opens is the desktop app's page on GitHub (onboarding.DownloadURL), in the system's browser. The app has no in-app browser.
- No account, no analytics, no crash reporting, no advertising and no third-party SDK: the app imports only NSGo and Kando's own packages.

## Privacy policy

The public privacy policy is the site's privacy page, https://kando.yldm.ai/privacy/ in English and /zh-CN/privacy/, /zh-TW/privacy/, /ja/privacy/ and /ko/privacy/ in the other languages (apps/site, the privacy… strings of its content files). Each App Store listing gives its language's page in privacy_url.txt; Google Play takes the policy's address in Play Console's App content › Privacy policy, which fastlane's supply does not upload, so enter https://kando.yldm.ai/privacy/ there. Change the page whenever this file's answers change.

## App Store: App Privacy

Data Not Collected. Apple counts data as collected when it is sent off the device where the developer or a third party can reach it; the app sends data only to the user's own paired computers. appstore/app_privacy_details.json holds this answer in the format fastlane's upload_app_privacy_details_to_app_store reads.

Permissions the app asks for, in apps/mobile/nsgo.yaml: Local Network (NSLocalNetworkUsageDescription), to reach the paired computer; notifications, only when the user turns on the review reminder in Settings, which is scheduled on the device.

Required-reason APIs, which nsgo build declares in the app's privacy manifest (PrivacyInfo.xcprivacy): NSGo's own, user defaults (CA92.1), file timestamps (C617.1) and the system's uptime (35F9.1), and disk space (85F4.1, apple.privacy in nsgo.yaml), which Settings › Offline Films reads to show the user the films' share of the device and the room left. The reading stays on the device, so App Privacy's answer does not change.

## Google Play: Data safety

- Does your app collect or share any of the required user data types? No. Data goes only from the phone to the user's own paired computer, over a connection encrypted end to end between the two (HTTPS pinned to the computer's certificate, or, through the storage relay, messages sealed with a key only the two hold, in the user's own bucket), which Google Play does not count as collection; nothing reaches the developer or a third party.
- Is all of the user data collected by your app encrypted in transit? Not asked once the answer above is No.
- Do you provide a way for users to request that their data be deleted? Not asked once the answer above is No. The user deletes films from the phone in the app, and unpairing a computer stops all transfers.

Permissions in the Android manifest: INTERNET and ACCESS_NETWORK_STATE (NSGo's runtime), to reach the computer; RECEIVE_BOOT_COMPLETED (NSGo's runtime), which schedules the review reminder again after a reboot; POST_NOTIFICATIONS (nsgo.yaml), for the review reminder.

