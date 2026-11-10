// Package quit draws the confirmation of quitting Kando while a job is running.
package quit

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Confirm asks, while *open, whether to quit although the job making film is still running, and reports true in the frame 退出 is clicked. 取消 is the default, so Enter and Escape keep Kando running and the job with it.
func Confirm(c *ui.Context, open *bool, film string) bool {
	if !*open {
		return false
	}
	return breeze.DestructiveAlertDialog(c, open, locales.QueueQuitWhileRunningTitle(), locales.QueueQuitWhileRunningMessage(film), 0, locales.QueueQuitWhileRunningQuit(), locales.QueueQuitWhileRunningCancel()) == 0
}
