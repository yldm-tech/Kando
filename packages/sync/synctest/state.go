package synctest

import (
	"context"
	"errors"
	"slices"
	"time"

	"kando/packages/sync"
)

// SetState changes records of library lib's learning state as the desktop would, stamped at at on the desktop (device "desktop") where they carry no stamp, and tells the phones waiting for events. A library listed (AddLibrary) starts with none.
func (d *Desktop) SetState(lib string, at time.Time, records ...sync.Record) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ledgerLocked(lib).Observe(records, func(sync.Record) bool { return false }, sync.Stamp{At: at, Device: "desktop"})
	d.desk.Journal.PublishState(lib)
}

// State is record kind id of library lib's learning state as the desktop has it.
func (d *Desktop) State(lib, kind, id string) (sync.Record, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ledgerLocked(lib).Record(kind, id)
}

// Refuse makes the desktop refuse each put that holds record kind id, the whole put, as Kando's refuses one with a record it cannot keep (sync.CodeInvalid).
func (d *Desktop) Refuse(kind, id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.refused == nil {
		d.refused = map[[2]string]bool{}
	}
	d.refused[[2]string{kind, id}] = true
}

// ledgerLocked is library lib's ledger, made the first time. d.mu is held.
func (d *Desktop) ledgerLocked(lib string) *sync.Ledger {
	if d.ledgers == nil {
		d.ledgers = map[string]*sync.Ledger{}
	}
	l, ok := d.ledgers[lib]
	if !ok {
		l = sync.NewLedger()
		d.ledgers[lib] = l
	}
	return l
}

// stateService is the stand-in's learning state as v2's calls read and write it: a library it lists has one, any other is not found.
type stateService struct{ d *Desktop }

// ledger is the ledger of library lib, which the desktop lists. d.mu is held.
func (s stateService) ledger(lib string) (*sync.Ledger, error) {
	if !slices.ContainsFunc(s.d.libs, func(l sync.Library) bool { return l.ID == lib }) {
		return nil, sync.Errorf(sync.CodeNotFound, "no library %q", lib)
	}
	return s.d.ledgerLocked(lib), nil
}

func (s stateService) GetState(_ context.Context, caller sync.Caller, req sync.StateGet) (sync.StateChanges, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	l, err := s.ledger(req.Library)
	if err != nil {
		return sync.StateChanges{}, err
	}
	out := l.Since(req.Library, req.Epoch, req.Since)
	out.Devices = map[string]string{"desktop": s.d.Name, caller.DeviceID: caller.Name}
	return out, nil
}

func (s stateService) PutState(_ context.Context, _ sync.Caller, req sync.StatePut) (sync.StatePutResult, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	l, err := s.ledger(req.Library)
	if err != nil {
		return sync.StatePutResult{}, err
	}
	for _, r := range req.Records {
		if s.d.refused[[2]string{r.Kind, r.ID}] {
			return sync.StatePutResult{}, sync.Errorf(sync.CodeInvalid, "%s %s cannot be kept", r.Kind, r.ID)
		}
	}
	_, conflicts := l.Put(req.Records, sync.NewCallID)
	out := sync.StatePutResult{Epoch: l.Epoch(), Rev: l.Rev(), Records: []sync.Record{}, Conflicts: conflicts}
	for _, r := range req.Records {
		if rec, ok := l.Record(r.Kind, r.ID); ok {
			out.Records = append(out.Records, rec)
		}
	}
	if out.Conflicts == nil {
		out.Conflicts = []sync.Conflict{}
	}
	s.d.desk.Journal.PublishState(req.Library)
	return out, nil
}

func (s stateService) ResolveConflict(_ context.Context, caller sync.Caller, req sync.ConflictResolve) (sync.StatePutResult, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	l, err := s.ledger(req.Library)
	if err != nil {
		return sync.StatePutResult{}, err
	}
	rec, c, err := l.Resolve(req.ID, req.Value, sync.Stamp{At: time.Now(), Device: caller.DeviceID})
	if errors.Is(err, sync.ErrNoConflict) {
		return sync.StatePutResult{}, sync.Errorf(sync.CodeNotFound, "no open conflict %q", req.ID)
	}
	if err != nil {
		return sync.StatePutResult{}, err
	}
	s.d.desk.Journal.PublishState(req.Library)
	return sync.StatePutResult{Epoch: l.Epoch(), Rev: l.Rev(), Records: []sync.Record{rec}, Conflicts: []sync.Conflict{c}}, nil
}
