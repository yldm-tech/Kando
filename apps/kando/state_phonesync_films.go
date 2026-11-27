package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"slices"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/locales"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
)

// Films through the storage (phonesync/films.go, yldm-tech/Kando#219): while the storage relay runs, Phone Sync puts every film of the libraries it serves in the storage for the phones away from this network, its video too unless 只同步字幕 is on, and the 存储用量 card of the 手机同步 page says what they take, which video goes up, cleans up the videos watched (自动清理 after a while, or by hand), and moves everything to another storage set up in 账号与上传 (换一个存储), handing the paired phones over so they follow it without pairing again.

// phoneFilmsHandoffFor is how long a move waits for the old storage to answer the phones' messages and take their new links.
const phoneFilmsHandoffFor = 2 * time.Minute

// phoneFilms is the films through the storage as the page shows them.
type phoneFilms struct {
	// subtitlesOnly is 只同步字幕 and cleanDays 自动清理存储里的视频 (0 for never), which settings.json keeps.
	subtitlesOnly bool
	cleanDays     int
	// status is where the films stand as the running server last told; cleaning is a cleanup under way and cleanErr why the last failed.
	status   phonesync.FilmsStatus
	cleaning bool
	cleanErr error
	// moving is a move to the store of kind moveTo, from moveFrom's, under way, whose films go up to it from the start numbered moveGen, 0 while the phones are handed over; moved is one done since Kando opened, moveErr why the last failed.
	moving, moved    bool
	moveTo, moveFrom string
	moveGen          int
	moveErr          error
}

// savedPhoneFilms is the films' part of settings.json: 只同步字幕, and how many days after a film was watched its video is cleaned up from the storage.
type savedPhoneFilms struct {
	PhoneRelaySubtitlesOnly bool `json:"phoneRelaySubtitlesOnly,omitempty"`
	PhoneRelayCleanDays     int  `json:"phoneRelayCleanDays,omitempty"`
}

// clean forgets a cleanup the page does not offer.
func (s *savedPhoneFilms) clean() {
	if !slices.Contains(settings.PhoneCleanDays, s.PhoneRelayCleanDays) {
		s.PhoneRelayCleanDays = 0
	}
}

// phoneFilmsHooks saves and takes back 只同步字幕 and the cleanup.
func (a *app) phoneFilmsHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedPhoneFilms = savedPhoneFilms{PhoneRelaySubtitlesOnly: a.phone.films.subtitlesOnly, PhoneRelayCleanDays: a.phone.films.cleanDays}
		},
		restore: func(saved savedSettings) {
			a.phone.films.subtitlesOnly, a.phone.films.cleanDays = saved.PhoneRelaySubtitlesOnly, saved.PhoneRelayCleanDays
		},
	}
}

// phoneFilmsCleanAfter is the cleanup as the server takes it.
func (a *app) phoneFilmsCleanAfter() time.Duration {
	return time.Duration(a.phone.films.cleanDays) * 24 * time.Hour
}

// phoneFilmsConfig is how the server of the start numbered gen shares the films through the store cfg, its status told back to the app as it changes. What the last server told is forgotten.
func (a *app) phoneFilmsConfig(gen int, cfg cloud.Store) *phonesync.FilmsConfig {
	a.phone.films.status, a.phone.films.cleaning, a.phone.films.cleanErr = phonesync.FilmsStatus{}, false, nil
	if a.phone.films.moving && a.phone.films.moveGen != 0 {
		// Phone Sync started again while the films go up to the new store: this start goes on with them.
		a.phone.films.moveGen = gen
	}
	update := a.update
	store := phoneFilmsStoreID(cfg)
	if a.phoneHostInUse() {
		// 看懂托管 is one bucket whatever key EveryAPI gives.
		store = "hosted"
	}
	return &phonesync.FilmsConfig{
		Videos:     a.phoneFilmsVideos(),
		CleanAfter: a.phoneFilmsCleanAfter(),
		Store:      store,
		Status: func(st phonesync.FilmsStatus) {
			update(func() {
				if gen != a.phone.gen {
					return
				}
				a.phone.films.status = st
				f := &a.phone.films
				// The move is done once a whole turn of the new start went through with every film up, not at a measure within the turn, before a video that failed says so.
				if f.moving && gen == f.moveGen && st.Settled && st.Measured && st.Uploading == "" && st.Waiting == 0 && st.Err == nil {
					f.moving, f.moved = false, true
				}
			})
		},
	}
}

// phoneFilmsStoreID names the store cfg by its kind, endpoint, region and bucket, without its keys, so a new key keeps what was put there and another bucket does not.
func phoneFilmsStoreID(cfg cloud.Store) string {
	sum := sha256.Sum256([]byte(storeKind(cfg.Kind) + "\n" + cfg.Endpoint + "\n" + cfg.Region + "\n" + cfg.Bucket))
	return hex.EncodeToString(sum[:8])
}

// phoneFilmsVideos reports whether the films go to the storage with their videos: not with 只同步字幕 on, nor while 看懂托管's storage is full (phoneHostFull).
func (a *app) phoneFilmsVideos() bool { return !a.phone.films.subtitlesOnly && !a.phoneHostFull() }

// phoneFilmsSetSubtitlesOnly turns 只同步字幕 on or off, saves it, and has a running relay take it up.
func (a *app) phoneFilmsSetSubtitlesOnly(on bool) {
	a.phone.films.subtitlesOnly = on
	a.Save()
	if srv := a.phone.server; srv != nil {
		srv.SetFilms(a.phoneFilmsVideos(), a.phoneFilmsCleanAfter())
	}
}

// phoneFilmsSetClean sets the cleanup to days after watching, 0 for never, saves it, and has a running relay take it up.
func (a *app) phoneFilmsSetClean(days int) {
	if !slices.Contains(settings.PhoneCleanDays, days) {
		return
	}
	a.phone.films.cleanDays = days
	a.Save()
	if srv := a.phone.server; srv != nil {
		srv.SetFilms(a.phoneFilmsVideos(), a.phoneFilmsCleanAfter())
	}
}

// phoneFilmsClean deletes the videos of watched from the storage, off the UI thread.
func (a *app) phoneFilmsClean(watched []settings.PhoneWatched) {
	srv := a.phone.server
	if srv == nil || a.phone.films.cleaning || len(watched) == 0 {
		return
	}
	refs := make([]phonesync.FilmRef, len(watched))
	for i, w := range watched {
		refs[i] = phonesync.FilmRef{Library: w.Library, ID: w.ID}
	}
	a.phone.films.cleaning, a.phone.films.cleanErr = true, nil
	gen, update := a.phone.gen, a.update
	srv.CleanVideos(refs, func(err error) {
		update(func() {
			if gen != a.phone.gen {
				return
			}
			a.phone.films.cleaning, a.phone.films.cleanErr = false, err
			if err != nil {
				log.Printf("phonesync: clean up the films' videos: %v", err)
			}
		})
	})
}

// phoneFilmsTargets are the kinds of store set up in 账号与上传 other than the one in use, which the relay can move to; none while it goes through 看懂托管, which the page's switch leaves.
func (a *app) phoneFilmsTargets() []string {
	if a.phoneHostInUse() {
		return nil
	}
	var out []string
	for _, kind := range []string{cloud.KindR2, cloud.KindS3, cloud.KindOSS, cloud.KindCOS, cloud.KindCompat} {
		if kind != a.storageKind() && storeReady(a.storeFor(kind)) {
			out = append(out, kind)
		}
	}
	return out
}

// phoneFilmsMove moves the relay and the films to the store of kind (phoneRelayMove), which then becomes the one in use in 账号与上传.
func (a *app) phoneFilmsMove(kind string) {
	if !slices.Contains(a.phoneFilmsTargets(), kind) {
		return
	}
	bucket, err := cloud.OpenBucket(a.storeFor(kind))
	a.phoneRelayMove(kind, a.storageKind(), phonesync.RelayBucket(bucket), nil, err, func() { a.cloud.storage = kind })
}

// phoneHostKind names 看懂托管 where a move names the store it goes to or comes from (phoneFilms.moveTo, moveFrom), beside the kinds of 账号与上传.
const phoneHostKind = "hosted"

// phoneRelayMove moves the relay and the films from the store named from to the bucket to, of the store named kind: the running server hands the phones over to it (phonesync.Server.Handoff), off the UI thread, with linksUntil when to's key is short-lived; then use makes it the store the relay runs in, and Phone Sync starts again with the relay there, which puts the films there anew. So phones away from this network follow without coming back to it: as 换一个存储 moves between the stores of 账号与上传, and as 看懂托管 is turned on or off (phoneHostSet). openErr is why to could not be opened, which ends the move before it starts. What is in the old store stays, for the user to delete.
func (a *app) phoneRelayMove(kind, from string, to phonesync.Bucket, linksUntil func(context.Context) (time.Time, error), openErr error, use func()) {
	f := &a.phone.films
	srv := a.phone.server
	if f.moving || srv == nil || a.phone.relaySig == "" {
		return
	}
	f.moved, f.moveErr, f.moveTo, f.moveFrom, f.moveGen = false, nil, kind, from, 0
	if openErr != nil {
		f.moveErr = openErr
		return
	}
	f.moving = true
	gen, update := a.phone.gen, a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), phoneFilmsHandoffFor)
		defer cancel()
		err := srv.Handoff(ctx, to, linksUntil)
		update(func() {
			if gen != a.phone.gen {
				// Phone Sync stopped or started again meanwhile: the move is not done.
				a.phone.films.moving = false
				return
			}
			if err != nil {
				log.Printf("phonesync: move the relay to %s: %v", kind, err)
				a.phone.films.moving, a.phone.films.moveErr = false, err
				return
			}
			use()
			a.Save()
			a.phoneRelayRestart()
			// The move goes on until the films are all in the new store, as the new start tells.
			a.phone.films.moving, a.phone.films.moveGen = true, a.phone.gen
		})
	}()
}

// phoneRelayStoreName names the store of kind as a move shows it: 看懂托管, or a store of 账号与上传 with its bucket.
func (a *app) phoneRelayStoreName(m locales.Messages, kind string, bucket bool) string {
	if kind == phoneHostKind {
		return m.SettingsPhoneRelayHosted()
	}
	if bucket {
		return m.SettingsPhoneRelayStoreName(uploadStoreName(m, kind), a.storeFor(kind).Bucket)
	}
	return uploadStoreName(m, kind)
}

// phoneFilmsStorage is the 存储用量 card as the page shows it, in m's language.
func (a *app) phoneFilmsStorage(m locales.Messages) settings.PhoneStorage {
	f := a.phone.films
	st := f.status
	out := settings.PhoneStorage{Measured: st.Measured, Videos: st.Videos, Lines: st.Lines, Mail: st.Mail, Films: st.Films, WithVideo: st.WithVideo,
		Uploading: st.Uploading, Waiting: st.Waiting, CleanDays: f.cleanDays, Cleaning: f.cleaning, Moving: f.moving, Moved: f.moved}
	if st.Size > 0 {
		out.Percent = int(st.Sent * 100 / st.Size)
	}
	cfg := a.currentStore()
	failure := func(err error) string {
		if a.phoneHostInUse() {
			return phoneHostFailure(m, err)
		}
		return uploadFailure(m, err, cfg)
	}
	if st.Err != nil {
		out.Err = failure(st.Err)
	}
	if f.cleanErr != nil {
		out.CleanErr = failure(f.cleanErr)
	}
	for _, w := range st.Watched {
		out.Watched = append(out.Watched, settings.PhoneWatched{Library: w.Library, ID: w.ID, Title: w.Title, Size: w.Size, At: w.At})
	}
	for _, kind := range a.phoneFilmsTargets() {
		out.Targets = append(out.Targets, settings.PhoneStore{Kind: kind, Name: uploadStoreName(m, kind)})
	}
	if f.moving || f.moved || f.moveErr != nil {
		out.MoveTo = a.phoneRelayStoreName(m, f.moveTo, false)
		out.MoveFrom = a.phoneRelayStoreName(m, f.moveFrom, true)
		out.Done, out.Total = st.WithVideo, st.WithVideo+st.Waiting
		switch {
		case f.moveErr == nil:
		case f.moveTo == phoneHostKind:
			out.MoveErr = phoneHostFailure(m, f.moveErr)
		default:
			out.MoveErr = uploadFailure(m, f.moveErr, a.storeFor(f.moveTo))
		}
	}
	return out
}
