package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/desktop"
	"kando/apps/kando/locales"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/relayhost"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// 看懂托管 (relayhost/): with Kando Plus, the storage relay goes through EveryAPI's hosted storage rather than the bucket of 账号与上传, its 看懂托管 switch on the 手机同步 page (settings.json's phoneRelayHosted) chosen. It asks EveryAPI with the key the AI service holds while that is EveryAPI: the key entered on the AI 服务 page or the everyapi program's sign-in (phoneHostKey); EveryAPI's in-app sign-in will give its key the same way once the translator holds it. EveryAPI decides everything: whether the account has Plus, the 7-day grace after it ends, the quota. Kando follows what it says: a full storage has the films go as subtitles only until there is room again (phoneHostFull), and writes stopped, once Plus and its grace ended, have the relay go back to the bucket of 账号与上传, or to the local network alone, until Plus is active again; nothing in the hosted storage is deleted.

// phoneHostAPI is EveryAPI's API the hosted relay asks, and phoneHostPoll how often the relay polls there (relayhost.Poll). Tests point them at a fake EveryAPI and shorten them.
var (
	phoneHostAPI  = relayhost.DefaultBase
	phoneHostPoll = relayhost.Poll
)

// phoneHostCheckFor is how long the 手机同步 page waits for EveryAPI to say whether the account has Plus.
const phoneHostCheckFor = 30 * time.Second

// phoneHost is 看懂托管 as the app keeps it.
type phoneHost struct {
	// on is 看懂托管 chosen, which settings.json keeps.
	on bool
	// lapsed is EveryAPI having stopped the account's writes, as once Kando Plus and the grace after it ended: the relay went back to the bucket of 账号与上传 until Plus is active again.
	lapsed bool
	// plus is Kando Plus as EveryAPI last said, plusKnown that it said; checking is a check under way, shown the page showing at the last afterChange, so it is checked as the page opens.
	plus      relayhost.Plus
	plusKnown bool
	checking  bool
	shown     bool
	// cred is the credential the hosted bucket made last asks with (phoneHostCred).
	cred phoneHostCred
	// state is what the hosted bucket of the running relay last found asking for its key, known once it asked.
	state      relayhost.State
	stateKnown bool
}

// savedPhoneRelayHost is 看懂托管's part of settings.json.
type savedPhoneRelayHost struct {
	PhoneRelayHosted bool `json:"phoneRelayHosted,omitempty"`
}

func (s *savedPhoneRelayHost) clean() {}

// phoneHostHooks saves and takes back the switch, and checks Plus as the 手机同步 page opens.
func (a *app) phoneHostHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedPhoneRelayHost = savedPhoneRelayHost{PhoneRelayHosted: a.phone.host.on}
		},
		restore: func(saved savedSettings) {
			a.phone.host.on = saved.PhoneRelayHosted
		},
		afterChange: a.phoneHostFollow,
	}
}

// phoneHostInUse reports whether the relay goes through 看懂托管 now: chosen, and its writes not stopped.
func (a *app) phoneHostInUse() bool { return a.phone.host.on && !a.phone.host.lapsed }

// phoneHostFull reports whether the hosted storage the relay runs in is full, so the films go as subtitles only.
func (a *app) phoneHostFull() bool { return a.phoneHostInUse() && a.phone.host.state.Full }

// phoneHostKey is the EveryAPI key the hosted relay asks with, as the AI service holds it now: none unless the service is EveryAPI; its key when one is entered, else the everyapi program's sign-in, asked on the goroutine that asks EveryAPI. UI thread only, as it reads the pipeline; what it returns is safe on any goroutine.
func (a *app) phoneHostKey() func(ctx context.Context, renew bool) (string, error) {
	tools, ok := a.pipeline.(media.Tools)
	none := func(context.Context, bool) (string, error) { return "", relayhost.ErrNoKey }
	if !ok || tools.Translator.BaseURL != media.EveryAPIBaseURL {
		return none
	}
	if key := tools.Translator.APIKey; key != "" {
		return func(context.Context, bool) (string, error) { return key, nil }
	}
	if credential := tools.Translator.Credential; credential != nil {
		return func(ctx context.Context, renew bool) (string, error) {
			endpoint, err := credential(ctx, renew)
			if err != nil {
				return "", errors.Join(relayhost.ErrNoKey, err)
			}
			return endpoint.APIKey, nil
		}
	}
	return none
}

// phoneHostClient asks EveryAPI with the key the AI service holds now. UI thread only.
func (a *app) phoneHostClient() *relayhost.Client {
	return &relayhost.Client{Base: phoneHostAPI, Key: a.phoneHostKey()}
}

// phoneHostCred is the AI service's credential the hosted relay asks EveryAPI with, as the pipeline holds it now: its endpoint, its key, whether a sign-in stands in for the key and which one, the everyapi program's or Kando's own (media.CredentialSource), so switching from one to the other asks with the new one. It is compared in memory with the one the running relay started with (phoneHost.cred), so Phone Sync starts again once it changes; nothing is derived from the key, and it is never saved or logged.
type phoneHostCred struct {
	base, key string
	signIn    bool
	source    media.CredentialSource
}

func (a *app) phoneHostCred() phoneHostCred {
	tools, _ := a.pipeline.(media.Tools)
	t := tools.Translator
	return phoneHostCred{base: t.BaseURL, key: t.APIKey, signIn: t.Credential != nil, source: t.CredentialSource}
}

// phoneHostBucket is the hosted bucket of the start numbered gen, which tells the app what EveryAPI says as it renews its key.
func (a *app) phoneHostBucket(gen int) *relayhost.Bucket {
	a.phone.host.cred = a.phoneHostCred()
	return relayhost.NewBucket(a.phoneHostClient(), nil, a.phoneHostTold(gen))
}

// phoneHostTold is what the hosted bucket of the start numbered gen tells, on its own goroutine, posted to the app when it says something else than the time before. Writes stopped, or EveryAPI saying the account has no Plus, has the relay go back to the bucket of 账号与上传 (phoneRelayFollow restarts it); the storage full, or no more, has the films go with their videos or without at once.
func (a *app) phoneHostTold(gen int) func(relayhost.State) {
	update := a.update
	told, last := false, ""
	return func(s relayhost.State) {
		text := strconv.FormatInt(s.Quota, 10) + "/" + strconv.FormatInt(s.Used, 10) + "/" + strconv.FormatBool(s.Writes) + "/" + strconv.FormatBool(s.Full)
		if s.Err != nil {
			text += "/" + s.Err.Error()
		}
		if told && text == last {
			return
		}
		told, last = true, text
		update(func() {
			if gen != a.phone.gen {
				return
			}
			videos := a.phoneFilmsVideos()
			a.phone.host.state, a.phone.host.stateKnown = s, true
			if errors.Is(s.Err, relayhost.ErrPlusRequired) || (s.Err == nil && !s.Writes) {
				log.Printf("phonesync: relay: Kando Plus ended; the hosted relay takes no more writes")
				a.phone.host.lapsed = true
			}
			if s.Full && !a.phone.host.lapsed {
				log.Printf("phonesync: relay: the hosted storage is full; the films go as subtitles only")
			}
			if srv := a.phone.server; srv != nil && videos != a.phoneFilmsVideos() {
				srv.SetFilms(a.phoneFilmsVideos(), a.phoneFilmsCleanAfter())
			}
		})
	}
}

// phoneHostSet chooses 看懂托管 (on), which only Kando Plus turns on, or the bucket of 账号与上传, and saves that. While the relay runs, it moves there as 换一个存储 does (phoneRelayMove), handing the phones over so those away from this network follow; with nowhere to hand them over to, as the bucket of 账号与上传 not set up, Phone Sync just starts again with the relay where it now goes.
func (a *app) phoneHostSet(on bool) {
	h := &a.phone.host
	if on == h.on || (on && !h.plus.Active) || a.phone.films.moving {
		return
	}
	use := func() {
		h := &a.phone.host
		h.on, h.lapsed = on, false
		h.state, h.stateKnown = relayhost.State{}, false
	}
	if on == a.phoneHostInUse() {
		// Turned off after its Plus ended: the relay already went back.
		use()
		a.Save()
		return
	}
	from := a.storageKind()
	if a.phoneHostInUse() {
		from = phoneHostKind
	}
	switch {
	case a.phone.server == nil || a.phone.relaySig == "":
	case on:
		b := a.phoneHostBucket(a.phone.gen)
		a.phoneRelayMove(phoneHostKind, from, b, b.LinksUntil, nil, use)
		return
	case storeReady(a.currentStore()):
		bucket, err := cloud.OpenBucket(a.currentStore())
		a.phoneRelayMove(a.storageKind(), from, phonesync.RelayBucket(bucket), nil, err, use)
		return
	}
	use()
	a.Save()
	a.phoneRelayRestart()
}

// phoneHostSubscribe opens EveryAPI's page where Kando Plus is subscribed to. The page checks Plus again as it opens next.
func (a *app) phoneHostSubscribe() {
	if a.cx == nil {
		return
	}
	desktop.OpenURL(a.cx, relayhost.PlansURL, nil)
	a.phone.host.shown = false
}

// phoneHostFollow checks Plus as the 手机同步 page opens with the relay on, or the relay is turned on there, as 看懂托管 shows then, off the UI thread, unless 离线模式 is on. It changes nothing a binding reads; the check's answer does, when it lands.
func (a *app) phoneHostFollow() bool {
	showing := a.phone.relay && a.prefs.open && a.SettingsSection() == settings.SectionPhone
	if showing == a.phone.host.shown {
		return false
	}
	a.phone.host.shown = showing
	if showing {
		a.phoneHostCheck()
	}
	return false
}

// phoneHostCheck asks EveryAPI whether the account has Plus. Plus active again after it lapsed has the relay go back to 看懂托管.
func (a *app) phoneHostCheck() {
	h := &a.phone.host
	if h.checking || a.privacy.offline {
		return
	}
	h.checking = true
	client, update := a.phoneHostClient(), a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), phoneHostCheckFor)
		defer cancel()
		plus, err := client.Plus(ctx)
		update(func() {
			h := &a.phone.host
			h.checking = false
			if err != nil {
				if !errors.Is(err, relayhost.ErrNoKey) {
					log.Printf("phonesync: Kando Plus: %v", err)
				}
				h.plusKnown = false
				h.plus = relayhost.Plus{}
				return
			}
			h.plus, h.plusKnown = plus, true
			if plus.Active && h.lapsed {
				a.phoneHostResume()
			}
		})
	}()
}

// phoneHostResume has the relay go back to 看懂托管 once Plus is active again after it lapsed. With the relay running in the bucket of 账号与上传, which still takes writes, it moves there as the switch does (phoneRelayMove), so the phones away from this network follow; a move that fails leaves it where it is, and the next check tries again. With the relay nowhere, as with no bucket of 账号与上传 set up, Phone Sync just starts again there (phoneRelayFollow). While another move hands the phones over, it waits for the next check rather than cutting that move short.
func (a *app) phoneHostResume() {
	if a.phone.films.moving {
		return
	}
	resume := func() {
		h := &a.phone.host
		h.lapsed = false
		h.state, h.stateKnown = relayhost.State{}, false
	}
	if a.phone.server == nil || a.phone.relaySig == "" {
		resume()
		return
	}
	b := a.phoneHostBucket(a.phone.gen)
	a.phoneRelayMove(phoneHostKind, a.storageKind(), b, b.LinksUntil, nil, resume)
}

// phoneHostStatus adds 看懂托管 to the relay as the 手机同步 page shows it, in m's language.
func (a *app) phoneHostStatus(m locales.Messages, r *settings.PhoneRelay) {
	h := a.phone.host
	// A key EveryAPI gave with writes is Plus too, or its grace, before the page checked.
	r.Hosted, r.HostedLapsed = h.on, h.on && h.lapsed
	r.Plus = h.plus.Active || (h.on && !h.lapsed && h.stateKnown && h.state.Err == nil && h.state.Writes)
	r.HostedQuota, r.HostedUsed = h.plus.Relay.QuotaBytes, h.plus.Relay.UsedBytes
	if h.stateKnown && h.state.Quota > 0 {
		r.HostedQuota, r.HostedUsed = h.state.Quota, h.state.Used
	}
	if !a.phoneHostInUse() {
		return
	}
	r.Store = m.SettingsPhoneRelayHosted()
	r.HostedFull = h.state.Full
}

// phoneHostFailure says why the hosted relay failed, in m's language, without EveryAPI's own words, which may carry a price or a link.
func phoneHostFailure(m locales.Messages, err error) string {
	switch {
	case errors.Is(err, relayhost.ErrQuotaExceeded):
		return m.SettingsPhoneRelayHostedFull()
	case errors.Is(err, relayhost.ErrNoKey), errors.Is(err, relayhost.ErrUnauthorized):
		return m.SettingsPhoneRelayHostedSignIn()
	case errors.Is(err, relayhost.ErrPlusRequired), errors.Is(err, relayhost.ErrWritesStopped):
		return m.SettingsPhoneRelayHostedLapsed()
	}
	switch cloud.Diagnose(err).Reason {
	case cloud.ReasonStalled, cloud.ReasonOffline, cloud.ReasonUnreachable:
		return uploadFailure(m, err, cloud.Store{})
	}
	return m.SettingsPhoneRelayHostedUnavailable()
}

// phoneRelayHosted is the hosted bucket of the start numbered gen as Phone Sync takes it: the bucket, how often to poll it and when the links signed now lapse.
func (a *app) phoneRelayHosted(gen int) (phonesync.Bucket, time.Duration, func(context.Context) (time.Time, error)) {
	b := a.phoneHostBucket(gen)
	return b, phoneHostPoll, b.LinksUntil
}
