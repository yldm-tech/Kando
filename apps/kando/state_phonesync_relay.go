package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/live"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	psync "kando/packages/sync"
)

// The storage relay (phonesync/relay.go, yldm-tech/Kando#219) carries Phone Sync's calls through the bucket of 账号与上传 when a phone is away from the computer's network. It runs while Phone Sync runs and its switch on the 手机同步 page (settings.json's phoneRelay) is on, in the store set up in 账号与上传, unless 离线模式 is on. Phone Sync starts again when the switch is turned, and phoneRelayRestartAfter after the store's settings or 离线模式 last changed, so the relay always runs in the store the page names.

// phoneRelayRestartAfter is how long after the store's settings last changed Phone Sync starts again in the new store, so typing a key does not restart it at each letter. Tests shorten it.
var phoneRelayRestartAfter = 2 * time.Second

// phoneRelaySig names the store the relay would run in now, "" for none: the relay off, 离线模式 on, or no store set up. Phone Sync keeps the one it started with (phoneUI.relaySig).
func (a *app) phoneRelaySig() string {
	if !a.phone.relay || a.privacy.offline {
		return ""
	}
	cfg := a.currentStore()
	if !storeReady(cfg) {
		return ""
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// phoneRelayBucket is the bucket the relay runs in, nil while the relay is off, 离线模式 is on or no store is set up. A store that does not open says why on the page.
func (a *app) phoneRelayBucket() psync.RelayBucket {
	if a.phoneRelaySig() == "" {
		if a.phone.relay && !a.privacy.offline {
			log.Printf("phonesync: relay: no storage is set up in Account & Upload")
		}
		return nil
	}
	bucket, err := cloud.OpenBucket(a.currentStore())
	if err != nil {
		log.Printf("phonesync: relay: %v", err)
		a.phone.relayErr = err
		return nil
	}
	return phonesync.RelayBucket(bucket)
}

// phoneRelayHealth is what the relay of the start numbered gen tells after each of its turns, on its own goroutine: posted to the app only when it says something else than the turn before, as most turns go as the last one did.
func (a *app) phoneRelayHealth(gen int) func(err error) {
	update := a.update
	told, last := false, ""
	return func(err error) {
		text := ""
		if err != nil {
			text = err.Error()
		}
		if told && text == last {
			return
		}
		told, last = true, text
		update(func() {
			if gen != a.phone.gen {
				return
			}
			a.phone.relayErr = err
			a.phone.relayOK = a.phone.relayOK || err == nil
		})
	}
}

// phoneRelaySet turns the relay on or off, saves that, and has a running Phone Sync take it up at once.
func (a *app) phoneRelaySet(on bool) {
	a.phone.relay = on
	a.Save()
	a.phoneRelayRestart()
}

// phoneRelayRestart starts a running Phone Sync again, so its relay runs in the store set now, or no more. The phones' connections drop and are made again at their next try.
func (a *app) phoneRelayRestart() {
	if a.phone.server == nil && !a.phone.starting {
		return
	}
	a.phoneSyncStop()
	a.phoneSyncStart()
}

// phoneRelayFollow restarts a running Phone Sync phoneRelayRestartAfter after the store the relay would run in stopped being the one it started with. 离线模式 turned on stops a running relay at once rather than after the wait, as it stops everything else under way over the network; it reports whether it did, which changes what the page reads. UI thread only.
func (a *app) phoneRelayFollow() bool {
	if a.phone.server == nil || a.phoneRelaySig() == a.phone.relaySig {
		return false
	}
	if a.privacy.offline && a.phone.relaySig != "" {
		a.phoneRelayRestart()
		return true
	}
	if a.phone.relayArmed || a.cx == nil {
		return false
	}
	a.phone.relayArmed = true
	gen := a.phone.gen
	a.cx.AfterFunc(phoneRelayRestartAfter, live.Do(a, func() {
		a.phone.relayArmed = false
		if gen == a.phone.gen && a.phone.server != nil && a.phoneRelaySig() != a.phone.relaySig {
			a.phoneRelayRestart()
		}
	}))
	return false
}

// phoneRelayStatus is the relay as the 手机同步 page shows it, in the interface's language.
func (a *app) phoneRelayStatus() settings.PhoneRelay {
	m := a.messages()
	cfg := a.currentStore()
	r := settings.PhoneRelay{On: a.phone.relay, Offline: a.privacy.offline, Working: a.phone.relayOK}
	if storeReady(cfg) {
		r.Store = m.SettingsPhoneRelayStoreName(uploadStoreName(m, cfg.Kind), cfg.Bucket)
	}
	if a.phone.relayErr != nil {
		r.Err = uploadFailure(m, a.phone.relayErr, cfg)
	}
	return r
}
