package main

import kcore "kando/packages/core"

// The open film's reference layer (film.Referencer): the captions its site made itself, which its job kept beside the video when the Add sheet asked for them (kcore.SubtitlesAuto), read as the film opens (readFilmInfo). The picture shows them above the film's own lines until the subtitle menu hides them, for every film while the app runs.

// Reference is the open film's reference layer, nil when it has none.
func (a *app) Reference() []kcore.Segment { return a.player.film.reference }

func (a *app) ShowReference() bool { return !a.player.referenceHidden }

func (a *app) SetShowReference(on bool) { a.player.referenceHidden = !on }
