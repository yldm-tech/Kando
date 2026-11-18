package sync

import "context"

// The computer's settings a paired phone reads and changes (yldm-tech/Kando#224): the language the computer translates subtitles into, which the phone's Settings › Translate into shows and sets, so the phone and the computer name one language rather than each its own. Only what a phone may change travels: the AI service's key never leaves the computer. Like every v2 call they go over any transport, the storage relay too.
const (
	// OpSettingsGet takes nothing and answers Settings.
	OpSettingsGet = "settings.get"
	// OpSettingsPut takes SettingsPut and answers Settings as they are once changed.
	OpSettingsPut = "settings.put"
)

// Settings are the computer's settings a phone sees.
type Settings struct {
	// Target is the language the computer translates subtitles into, as a BCP 47 tag ("zh-Hans", "en", "ja"), "" when the computer's language is none a tag names, as one the user typed in; TargetName is how the computer names it, as its settings show it, which a phone shows for a Target it does not list.
	Target     string `json:"target"`
	TargetName string `json:"target_name,omitempty"`
}

// SettingsPut changes the computer's settings: Target, when not "", is the language to translate into, a BCP 47 tag. A tag the computer does not translate into answers CodeInvalid.
type SettingsPut struct {
	Target string `json:"target,omitempty"`
}

// SettingsService is what a desktop does for the settings' calls, whichever transport carries them. Its errors are as Service's.
type SettingsService interface {
	Settings(ctx context.Context, caller Caller) (Settings, error)
	PutSettings(ctx context.Context, caller Caller, req SettingsPut) (Settings, error)
}

// answerSettings runs a call of the settings.
func (d *Desk) answerSettings(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Settings == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not share its settings")
	}
	if call.Op == OpSettingsGet {
		return d.Settings.Settings(ctx, caller)
	}
	var req SettingsPut
	if err := args(call, &req); err != nil {
		return nil, err
	}
	return d.Settings.PutSettings(ctx, caller, req)
}

// Settings are the computer's settings a phone sees.
func (c *Client) Settings(ctx context.Context) (Settings, error) {
	var out Settings
	err := c.Call(ctx, OpSettingsGet, nil, &out)
	return out, err
}

// PutSettings changes the computer's settings, and answers them as they are after.
func (c *Client) PutSettings(ctx context.Context, req SettingsPut) (Settings, error) {
	var out Settings
	err := c.Call(ctx, OpSettingsPut, req, &out)
	return out, err
}
