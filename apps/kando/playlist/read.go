// Package playlist is how a playlist or a channel link becomes one job per video: Read lists its videos with the pipeline's yt-dlp (media.Lister), and Confirm is the dialog that says how many there are and asks before they are queued. The library's link field, the browser's Download Video and the message bars' Add Whole Playlist open it; the command line and the MCP server list the videos through the app's socket (remote.MethodLists) and ask in their own way. The app keeps the dialog's state and queues the videos (state_playlist.go).
package playlist

import (
	"context"
	"strings"

	"kando/packages/core"
	"kando/packages/media"
)

// ChannelLimit is how many of a channel's videos are listed, its most recent: a channel may hold thousands, so the dialog, kando add and the MCP server say that only these are added.
const ChannelLimit = 50

// List is what Read found behind one link: the link, whether it is a channel (whose ChannelLimit most recent videos are listed) rather than a playlist, and its videos.
type List struct {
	Source  string
	Channel bool
	media.Listing
}

// Read lists the videos of each of sources, playlist and channel links (core.Link.IsList), one after another, with lister: a playlist whole, a channel's ChannelLimit most recent. It stops at the first that fails, with its error, and when ctx ends.
func Read(ctx context.Context, lister media.Lister, sources []string) ([]List, error) {
	var lists []List
	for _, source := range sources {
		list, err := ReadOne(ctx, lister, source)
		if err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	return lists, nil
}

// ReadOne lists the videos of source as Read does.
func ReadOne(ctx context.Context, lister media.Lister, source string) (List, error) {
	link, _ := core.ParseLink(strings.TrimSpace(source))
	list := List{Source: strings.TrimSpace(source), Channel: link.Kind == core.LinkChannel}
	limit := 0
	if list.Channel {
		limit = ChannelLimit
	}
	listing, err := lister.ListVideos(ctx, list.Source, limit)
	if err != nil {
		return List{}, err
	}
	list.Listing = listing
	return list, nil
}

// Split sorts sources into the ones that are a playlist or a channel (core.Link.IsList), each once, and the others, in their order. Sources are trimmed; empty ones are kept among the others, for the precheck to refuse.
func Split(sources []string) (videos, lists []string) {
	seen := map[string]bool{}
	for _, source := range sources {
		source = strings.TrimSpace(source)
		link, ok := core.ParseLink(source)
		if !ok || !link.IsList() {
			videos = append(videos, source)
			continue
		}
		if !seen[link.URL] {
			seen[link.URL] = true
			lists = append(lists, source)
		}
	}
	return videos, lists
}
