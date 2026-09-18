package discoveringest

import "morgenblau/internal/lexicon"

// Collections are the reader-network collections Jetstream is filtered to: the
// eleven that reduce into per-source trending signals, plus the two follow
// lexicons behind People trending. app.bsky.graph.follow is deliberately absent.
// SPEC <lexicons> External Lexicons, SPEC <discovery>.
var Collections = []string{
	"blue.morgen.feed.subscription",
	"blue.morgen.feed.save",
	"blue.morgen.feed.share",
	"app.skyreader.feed.subscription",
	"app.skyreader.feed.saved",
	"app.skyreader.social.share",
	"at.glean.subscription",
	"at.glean.like",
	"site.standard.publication",
	"site.standard.graph.subscription",
	"site.standard.graph.recommend",
	lexicon.Follow,
	"app.skyreader.social.follow",
}

var trackedCollections = func() map[string]struct{} {
	out := make(map[string]struct{}, len(Collections))
	for _, c := range Collections {
		out[c] = struct{}{}
	}
	return out
}()

func tracked(collection string) bool {
	_, ok := trackedCollections[collection]
	return ok
}
