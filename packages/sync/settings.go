package sync

import "context"

// The computer's settings a paired phone reads and changes (yldm-tech/Kando#224): the language the computer translates subtitles into, which the phone's Settings › Translate into shows and sets, so the phone and the computer name one language rather than each its own; whether translating sends the lines before a batch with it, which the phone's page of the computer sets; and the AI service the computer translates with, which the phone only shows. Only what a phone may see travels: the AI service's address and key never leave the computer. Like every v2 call they go over any transport, the storage relay too. OpSettingsPut changes the computer (Mutates), so a read-only device (Caller.ReadOnly) is refused it; OpSettingsGet only reads.
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
	// Service is the AI service the computer translates and explains with, as its settings name it in its own language ("OpenAI", "DeepSeek", its word for a custom endpoint), and Model the model it uses there; both "" while no service is set up. Neither is the service's address.
	Service string `json:"service,omitempty"`
	Model   string `json:"model,omitempty"`
	// Context reports whether the computer sends the lines before a batch with it when it translates, for a smoother translation; nil from a computer that does not share it.
	Context *bool `json:"context,omitempty"`
}

// SettingsPut changes the computer's settings: Target, when not "", is the language to translate into, a BCP 47 tag, and Context, when not nil, whether translating sends the lines before a batch with it. A tag the computer does not translate into answers CodeInvalid.
type SettingsPut struct {
	Target  string `json:"target,omitempty"`
	Context *bool  `json:"context,omitempty"`
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
