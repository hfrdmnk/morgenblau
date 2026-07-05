// Package lexicon holds the blue.morgen NSIDs and record $type discriminators
// shared between the api (writer) and sync (reader) packages. Declaring them
// once keeps the two sides from drifting: a mismatch would make the reconciler
// silently skip records the app itself wrote.
package lexicon

// blue.morgen collection NSIDs.
const (
	Subscription = "blue.morgen.feed.subscription"
	Save         = "blue.morgen.feed.save"
	Share        = "blue.morgen.feed.share"
)

// Source union $type discriminators on a blue.morgen.feed.subscription record.
const (
	SourceRSS      = Subscription + "#rssFeed"
	SourceStandard = Subscription + "#standardPublication"
)
