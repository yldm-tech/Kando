package sync

import (
	"context"
	"time"
)

// The computer's subscriptions a paired phone follows and changes (yldm-tech/Kando#221): the channels, uploaders' spaces, collections and playlists the computer checks for new videos, which the phone's Library lists in a row of their own. A phone lists them (OpSubscriptions), subscribes the computer to a list link (OpSubscribe), turns a subscription's auto-download on or off and pauses or resumes its checks (OpSubscriptionSet), sends the new videos waiting on one to the queue (OpSubscriptionTake) and unsubscribes (OpUnsubscribe). The checks themselves stay the computer's: it runs them on its own schedule, and a phone sees what they found at its next list. A subscription of a locked library is neither listed nor acted on: to a phone it is not found. Like every v2 call they go over any transport, the storage relay too; all but OpSubscriptions change the computer (Mutates), so a read-only device (Caller.ReadOnly) is refused them.
const (
	// OpSubscriptions takes nothing and answers SubscriptionList.
	OpSubscriptions = "subscriptions.list"
	// OpSubscribe takes SubscriptionAdd and answers Subscribed. The computer reads the list first, which takes a while for a long one: a phone gives the call SubscribeWait.
	OpSubscribe = "subscriptions.add"
	// OpSubscriptionSet takes SubscriptionSet and answers the Subscription as it is once changed.
	OpSubscriptionSet = "subscriptions.set"
	// OpSubscriptionTake takes SubscriptionTake and answers the Subscription once its videos are queued.
	OpSubscriptionTake = "subscriptions.take"
	// OpUnsubscribe takes SubscriptionRef and answers nothing; the videos the subscription added stay.
	OpUnsubscribe = "subscriptions.remove"
)

// SubscribeWait is how long a phone waits for OpSubscribe, which reads the whole list with the computer's downloader before it answers.
const SubscribeWait = 3 * time.Minute

// SubscriptionList is the subscriptions of the libraries the computer serves, in the order the computer lists them.
type SubscriptionList struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

// Subscription is a list the computer checks for new videos.
type Subscription struct {
	ID string `json:"id"`
	// Library is the ID of the library its videos are added to (Library.ID).
	Library string `json:"library"`
	// Name is what the computer calls it; Source its link, as it was pasted.
	Name   string `json:"name"`
	Source string `json:"source"`
	// Kind is what it follows (SubscriptionChannel…), Platform the site it is on (PlatformYouTube, PlatformBilibili), "" for another site.
	Kind     string `json:"kind"`
	Platform string `json:"platform,omitempty"`
	// Auto is a subscription whose new videos are queued by themselves, at most AddPerCheck of them each check, the others waiting; otherwise every new video waits for the user (Waiting).
	Auto bool `json:"auto"`
	// AddPerCheck is the most new videos a check queues by itself.
	AddPerCheck int `json:"add_per_check,omitempty"`
	// Paused is a subscription the user stopped checking; Gone one whose site no longer has the list, which the computer still looks for once a day.
	Paused bool `json:"paused,omitempty"`
	Gone   bool `json:"gone,omitempty"`
	// Checked is when it was last checked, zero before; Error is why that check failed, in the computer's words in English, "" when it worked.
	Checked time.Time `json:"checked,omitzero"`
	Error   string    `json:"error,omitempty"`
	// Waiting are the new videos waiting for the user to add or skip, oldest first; Added is how many videos it has added to the queue.
	Waiting []SubscriptionVideo `json:"waiting"`
	Added   int                 `json:"added"`
}

// The kinds of Subscription: a channel, a bilibili uploader's space, a bilibili collection, or a playlist of any other site.
const (
	SubscriptionChannel    = "channel"
	SubscriptionSpace      = "space"
	SubscriptionCollection = "collection"
	SubscriptionPlaylist   = "playlist"
)

// The platforms of Subscription.
const (
	PlatformYouTube  = "youtube"
	PlatformBilibili = "bilibili"
)

// SubscriptionVideo is a video a check found: its link, its title and how long it plays, 0 when the site did not say.
type SubscriptionVideo struct {
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// SubscriptionAdd subscribes the computer's open library to Link, a channel, a space, a collection or a playlist. A link that is none of them answers CodeInvalid; without an open library, or with it locked, CodeNotReady.
type SubscriptionAdd struct {
	Link string `json:"link"`
}

// Subscribed is the answer to SubscriptionAdd: the subscription made, or with Existing set the one the library follows the list with already.
type Subscribed struct {
	Subscription Subscription `json:"subscription"`
	Existing     bool         `json:"existing,omitempty"`
}

// SubscriptionSet changes subscription ID: Auto, when not nil, turns its auto-download on or off; Paused, when not nil, pauses or resumes its checks.
type SubscriptionSet struct {
	ID     string `json:"id"`
	Auto   *bool  `json:"auto,omitempty"`
	Paused *bool  `json:"paused,omitempty"`
}

// SubscriptionTake sends the videos waiting on subscription ID at Links to the queue, all of them for none. A video that could not be queued stays waiting, and the answer is CodeNotReady with why.
type SubscriptionTake struct {
	ID    string   `json:"id"`
	Links []string `json:"links,omitempty"`
}

// SubscriptionRef names a subscription.
type SubscriptionRef struct {
	ID string `json:"id"`
}

// SubscriptionService is what a desktop does for the subscriptions' calls, whichever transport carries them. Its errors are as Service's.
type SubscriptionService interface {
	Subscriptions(ctx context.Context, caller Caller) (SubscriptionList, error)
	Subscribe(ctx context.Context, caller Caller, req SubscriptionAdd) (Subscribed, error)
	SetSubscription(ctx context.Context, caller Caller, req SubscriptionSet) (Subscription, error)
	TakeSubscription(ctx context.Context, caller Caller, req SubscriptionTake) (Subscription, error)
	Unsubscribe(ctx context.Context, caller Caller, req SubscriptionRef) error
}

// answerSubscriptions runs a call of the subscriptions.
func (d *Desk) answerSubscriptions(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Subscriptions == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not share its subscriptions")
	}
	switch call.Op {
	case OpSubscriptions:
		return d.Subscriptions.Subscriptions(ctx, caller)
	case OpSubscribe:
		var req SubscriptionAdd
		if err := args(call, &req); err != nil {
			return nil, err
		}
		return d.Subscriptions.Subscribe(ctx, caller, req)
	case OpSubscriptionSet:
		var req SubscriptionSet
		if err := args(call, &req); err != nil {
			return nil, err
		}
		return d.Subscriptions.SetSubscription(ctx, caller, req)
	case OpSubscriptionTake:
		var req SubscriptionTake
		if err := args(call, &req); err != nil {
			return nil, err
		}
		return d.Subscriptions.TakeSubscription(ctx, caller, req)
	}
	var ref SubscriptionRef
	if err := args(call, &ref); err != nil {
		return nil, err
	}
	return struct{}{}, d.Subscriptions.Unsubscribe(ctx, caller, ref)
}

// Subscriptions are the computer's subscriptions.
func (c *Client) Subscriptions(ctx context.Context) (SubscriptionList, error) {
	var out SubscriptionList
	err := c.Call(ctx, OpSubscriptions, nil, &out)
	return out, err
}

// Subscribe subscribes the computer's open library to link.
func (c *Client) Subscribe(ctx context.Context, link string) (Subscribed, error) {
	var out Subscribed
	err := c.Call(ctx, OpSubscribe, SubscriptionAdd{Link: link}, &out)
	return out, err
}

// SetSubscription changes a subscription, and answers it as it is after.
func (c *Client) SetSubscription(ctx context.Context, req SubscriptionSet) (Subscription, error) {
	var out Subscription
	err := c.Call(ctx, OpSubscriptionSet, req, &out)
	return out, err
}

// TakeSubscription sends the videos waiting on subscription id at links, all of them for nil, to the queue, and answers the subscription as it is after.
func (c *Client) TakeSubscription(ctx context.Context, id string, links []string) (Subscription, error) {
	var out Subscription
	err := c.Call(ctx, OpSubscriptionTake, SubscriptionTake{ID: id, Links: links}, &out)
	return out, err
}

// Unsubscribe removes subscription id from the computer.
func (c *Client) Unsubscribe(ctx context.Context, id string) error {
	return c.Call(ctx, OpUnsubscribe, SubscriptionRef{ID: id}, nil)
}
