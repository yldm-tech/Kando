package main

import (
	"errors"
	"log"

	"kando/apps/kando/storage"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The storage locations' passwords, private keys and passphrases are kept in the system's secure store (ns.Secrets), each under the account <location ID>.<key> of the partition locationSecretsPartition, rather than in settings.json (docs/media-sources.md 17.2). settings.json holds a location's secrets only where the store did not take them (locationsUI.plain), as a Linux session without a Secret Service has no store: the settings page says so (LocationSecretsInFile). A settings.json written by an earlier version holds every location's secrets; they stay there until the store has taken them, so a store that fails never loses one.

// locationSecretsPartition is the partition of the system's secure store the locations' secrets are kept in (ns.Scope.ProvideSecrets).
const locationSecretsPartition = "storage"

// locationSecretKeys are the secrets of a location, by the key they are kept under in the secure store.
var locationSecretKeys = []string{"password", "privateKey", "passphrase"}

// locationSecretAccount is the account of the secure store a location's secret key is kept under.
func locationSecretAccount(id, key string) string { return id + "." + key }

// locationSecretValues are r's secrets by key, the empty ones among them.
func locationSecretValues(r storage.Remote) map[string]string {
	return map[string]string{"password": r.Password, "privateKey": r.PrivateKey, "passphrase": r.Passphrase}
}

// locationHasSecrets reports whether r holds any secret.
func locationHasSecrets(r storage.Remote) bool {
	return r.Password != "" || r.PrivateKey != "" || r.Passphrase != ""
}

// locationSameSecrets reports whether a and b hold the same secrets.
func locationSameSecrets(a, b storage.Remote) bool {
	return a.Password == b.Password && a.PrivateKey == b.PrivateKey && a.Passphrase == b.Passphrase
}

// locationSetSecrets puts values, by key, into r.
func locationSetSecrets(r *storage.Remote, values map[string]string) {
	r.Password, r.PrivateKey, r.Passphrase = values["password"], values["privateKey"], values["passphrase"]
}

// locationSecretsWrite keeps values as the secrets of the location id in store: each one set, an empty one deleted, so the store holds what the location holds and nothing it no longer does.
func locationSecretsWrite(store ns.Secrets, id string, values map[string]string) error {
	for _, key := range locationSecretKeys {
		account := locationSecretAccount(id, key)
		var err error
		if v := values[key]; v != "" {
			err = store.Set(account, v)
		} else {
			err = store.Delete(account)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// locationSecretsRead is what store keeps for the location id, by key, a key it keeps nothing for left empty.
func locationSecretsRead(store ns.Secrets, id string) (map[string]string, error) {
	values := map[string]string{}
	for _, key := range locationSecretKeys {
		v, _, err := store.Get(locationSecretAccount(id, key))
		if err != nil {
			return nil, err
		}
		values[key] = v
	}
	return values, nil
}

// locationSecretStore is the partition of the system's secure store the locations' secrets are kept in, or the one a test set (locationsUI.secrets); without the app's scope, as in a test that starts no app, one that fails with ns.ErrSecretsUnavailable.
func (a *app) locationSecretStore() ns.Secrets {
	if a.locations.secrets != nil {
		return a.locations.secrets()
	}
	if a.cx == nil {
		return ns.Secrets{}
	}
	inner := a.cx.Child()
	var secrets ns.Secrets
	inner.Run(func() {
		inner.ProvideSecrets(locationSecretsPartition)
		secrets = inner.Secrets()
	})
	inner.Dispose()
	return secrets
}

// locationSecretsDo runs op off the engine's goroutine, as the system may ask the user before the store answers, after every op queued before it has run, so a removal's forgetting and a later save of the same ID reach the store in the order they were made; what op returns then runs on the engine's goroutine. secretsBusy counts the ops not done yet.
func (a *app) locationSecretsDo(op func() (done func())) {
	l := &a.locations
	prev, next := l.secretsTail, make(chan struct{})
	l.secretsTail = next
	l.secretsBusy++
	update := a.update
	go func() {
		if prev != nil {
			<-prev
		}
		done := op()
		close(next)
		update(func() {
			l.secretsBusy--
			if done != nil {
				done()
			}
		})
	}()
}

// locationSecretsKeep writes the secrets the location id holds to the secure store. Until the store took them, settings.json keeps them for one plain: written by an earlier version, refused by the store before, or typed anew in the dialog (saveLocationDraft). Once taken, settings.json is saved without them; one the store refuses is saved with them (locationSecretsKept).
func (a *app) locationSecretsKeep(id string) {
	i := a.locationIndex(id)
	if i < 0 {
		return
	}
	l := &a.locations
	l.secretGen[id]++
	gen := l.secretGen[id]
	values := locationSecretValues(l.list[i].Remote)
	store := a.locationSecretStore()
	a.locationSecretsDo(func() func() {
		err := locationSecretsWrite(store, id, values)
		return func() { a.locationSecretsKept(id, gen, err) }
	})
}

// locationSecretsKept records where the secrets of the write gen of the location id are kept, unless a later change of its secrets or its removal took its place, and saves settings.json when that changed: without them once the store took them, with them when it refused them.
func (a *app) locationSecretsKept(id string, gen int, err error) {
	l := &a.locations
	if l.secretGen[id] != gen || a.locationIndex(id) < 0 {
		return
	}
	was := l.plain[id]
	if err != nil {
		if errors.Is(err, ns.ErrSecretsUnavailable) {
			log.Printf("storage location: no secure store, so its secrets stay in settings.json")
		} else {
			log.Printf("storage location: the secure store did not take its secrets, so they stay in settings.json: %v", err)
		}
		l.plain[id] = true
	} else {
		delete(l.plain, id)
	}
	if was == l.plain[id] {
		return
	}
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().AccountLocationSaveFailed()
	}
}

// locationSecretsForget forgets the secrets of the location id, just removed, in the secure store. A failure leaves them there, as nothing reads them any more.
func (a *app) locationSecretsForget(id string) {
	l := &a.locations
	l.secretGen[id]++
	delete(l.plain, id)
	store := a.locationSecretStore()
	a.locationSecretsDo(func() func() {
		if err := locationSecretsWrite(store, id, nil); err != nil && !errors.Is(err, ns.ErrSecretsUnavailable) {
			log.Printf("storage location: forgetting its secrets failed: %v", err)
		}
		return nil
	})
}

// locationSecretsLoad runs once the app's scope exists (ready): it reads from the secure store the secrets of each location settings.json keeps without them, and moves into the store those of each location settings.json keeps with them, as an earlier version saved them.
func (a *app) locationSecretsLoad() {
	l := &a.locations
	store := a.locationSecretStore()
	for _, loc := range l.list {
		id := loc.ID
		if l.plain[id] {
			a.locationSecretsKeep(id)
			continue
		}
		gen := l.secretGen[id]
		a.locationSecretsDo(func() func() {
			values, err := locationSecretsRead(store, id)
			return func() { a.locationSecretsLoaded(id, gen, values, err) }
		})
	}
}

// locationSecretsLoaded gives the location id the secrets read from the store, unless its secrets changed or it went away meanwhile. A store that could not be read leaves it without them: settings.json is not touched, so they are read again at the next start.
func (a *app) locationSecretsLoaded(id string, gen int, values map[string]string, err error) {
	l := &a.locations
	i := a.locationIndex(id)
	if l.secretGen[id] != gen || i < 0 {
		return
	}
	if err != nil {
		log.Printf("storage location: reading its secrets from the secure store failed: %v", err)
		return
	}
	locationSetSecrets(&l.list[i].Remote, values)
}

// locationSecretsInFile reports whether settings.json keeps the secrets of any location, as the secure store did not take them.
func (l locationsUI) locationSecretsInFile() bool {
	for _, loc := range l.list {
		if l.plain[loc.ID] && locationHasSecrets(loc.Remote) {
			return true
		}
	}
	return false
}
