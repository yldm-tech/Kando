package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// auditName is the audit log's name in the plugins folder; auditMax the most bytes it grows to before it is moved to auditName.1, the one before it, which goes.
const (
	auditName = "audit.log"
	auditMax  = 512 << 10
)

// The events of the audit log (Audit).
const (
	AuditInstall   = "install"
	AuditUninstall = "uninstall"
	AuditGrant     = "grant"
	AuditEnable    = "enable"
	AuditDisable   = "disable"
	AuditStopped   = "stopped"
	AuditRevoked   = "revoked"
	AuditOpenURL   = "open-url"
)

// Audit adds a line to the audit log of root, audit.log: the time, the event, the plugin's id and what it was about, such as the permissions granted, the reason a plugin was turned off or the answer to a confirmation; never what a plugin read or sent. The line goes through scrub first, which hides what reads as a secret or a path of the user's (the app passes feedback.Scrubber's Clean, which this package, imported by no interface package, cannot import itself), and is one line whatever detail holds. Past auditMax bytes the log moves to audit.log.1 and starts again.
func Audit(root string, scrub func(string) string, at time.Time, event, id, detail string) error {
	line := fmt.Sprintf("%s %s %s %s", at.UTC().Format(time.RFC3339), event, id, detail)
	line = strings.Join(strings.Fields(scrub(line)), " ") + "\n"
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	path := filepath.Join(root, auditName)
	if info, err := os.Stat(path); err == nil && info.Size()+int64(len(line)) > auditMax {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(line)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// AuditPath is the audit log of root.
func AuditPath(root string) string { return filepath.Join(root, auditName) }
