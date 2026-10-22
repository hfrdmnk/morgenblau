// Package lexicon holds the blue.morgen NSIDs and $type discriminators shared between api (writer) and sync (reader), so the two sides can't drift silently.
package lexicon

// blue.morgen collection NSIDs.
const (
	Subscription = "blue.morgen.feed.subscription"
	Save         = "blue.morgen.feed.save"
	Share        = "blue.morgen.feed.share"
	Follow       = "blue.morgen.graph.follow"
)

// Source union $type discriminators on a blue.morgen.feed.subscription record.
const (
	SourceRSS      = Subscription + "#rssFeed"
	SourceStandard = Subscription + "#standardPublication"
)
