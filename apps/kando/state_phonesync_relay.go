package main

import (
	"log"

	"kando/apps/kando/cloud"
	"kando/apps/kando/phonesync"
	psync "kando/packages/sync"
)

// The storage relay (phonesync/relay.go, yldm-tech/Kando#219) carries Phone Sync's calls through the bucket of 账号与上传 when a phone is away from the computer's network. It is off unless settings.json's phoneRelay is on, and has no switch on the 手机同步 page yet, since no phone reads it yet; it runs while Phone Sync runs, in the store set when Phone Sync started.

// phoneRelayBucket is the bucket the relay runs in, nil while the relay is off or no store is set up.
func (a *app) phoneRelayBucket() psync.RelayBucket {
	if !a.phone.relay {
		return nil
	}
	cfg := a.currentStore()
	if !storeReady(cfg) {
		log.Printf("phonesync: relay: no storage is set up in Account & Upload")
		return nil
	}
	bucket, err := cloud.OpenBucket(cfg)
	if err != nil {
		log.Printf("phonesync: relay: %v", err)
		return nil
	}
	return phonesync.RelayBucket(bucket)
}
