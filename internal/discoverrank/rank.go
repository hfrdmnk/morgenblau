// Package discoverrank is the personal-sources ranking engine for Discover.
// Pure, no I/O: callers gather candidates and the excluded-key set; this package only scores, orders, and explains them. SPEC <discovery> Personal ranking.
package discoverrank

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sort"
	"time"

	"morgenblau/internal/discoverlang"
)

// Tier is the trust level a signal carries: self > reader-network follow > Bluesky/Tangled follow. SPEC <discovery> Trust tiers.
type Tier int

const (
	TierSelf Tier = iota
	TierStrong
	TierWeak
)

// Network names the adjacent social graph a weak-tier follower's signal came from. Reason formatting only, never affects scoring.
type Network string

const (
	NetworkBluesky Network = "bluesky"
	NetworkTangled Network = "tangled"
)

// SelfSourceApp names which foreign reader app a self-tier signal's record came from. Reason formatting only; set only when Tier == TierSelf. SPEC <discovery> The user's own foreign records.
type SelfSourceApp string

const (
	SelfSourceSkyreader SelfSourceApp = "skyreader"
	SelfSourceGlean     SelfSourceApp = "glean"
)

// SignalKind is the contribution strength ordering: author > subscribe > share > save; higher ordinal wins on ties. SPEC <discovery> Signal ordering.
type SignalKind int

const (
	SignalSave SignalKind = iota
	SignalShare
	SignalSubscribe
	SignalAuthor
)

// String names the signal kind, for reason formatting on the wire.
func (k SignalKind) String() string {
	switch k {
	case SignalAuthor:
		return "author"
	case SignalSubscribe:
		return "subscribe"
	case SignalShare:
		return "share"
	case SignalSave:
		return "save"
	default:
		return ""
	}
}

// Signal is one raw contribution: a kind plus the anchor time recency lean and decay are computed against. Zero time means unknown recency and degrades to neutral, never inflated.
type Signal struct {
	Kind SignalKind
	At   time.Time
}

// SPEC <discovery> Trust tiers. SelfWeight exceeds StrongWeight so self outranks strong at equal signal kind. WeakCapStrongEquivalents sits a full score band below two strong subscribers, so band-tie rotation can never lift a capped weak pile above genuine depth.
const (
	SelfWeight               = 1.5
	StrongWeight             = 1.0
	WeakWeight               = 0.25
	WeakCapStrongEquivalents = 1.8
)

// Signal base weights: gaps between adjacent kinds exceed the max recency lean/decay range, so equal-recency ordering by kind never inverts. SPEC <discovery> Signal ordering.
const (
	AuthorBaseWeight    = 1.5
	SubscribeBaseWeight = 1.0
	ShareBaseWeight     = 0.7
	SaveBaseWeight      = 0.4
)

// Recency lean on standing signals (author, subscribe): up to +15% at age zero, decaying to +0% over a 30-day half-life. SPEC <discovery>: newer/active outranks older/dormant.
const (
	standingLeanMax      = 0.15
	standingLeanHalfLife = 30 * 24 * time.Hour
)

// HN-gravity decay on reaction signals (share, save). Constants match HN's own tuning; days not hours, to fit this product's daily cadence. SPEC <discovery>.
const (
	reactionGravity           = 1.8
	reactionGravityOffsetDays = 1.5
)

// scoreBandWidth is the near-tie band the seeded shuffle rotates within. It equals the largest score swing the subscribe recency lean alone can produce, so every deliberate ordering gap (signal kinds, an extra follower, the weak-cap margin) spans at least one full band and never shares one with lean-scale noise.
const scoreBandWidth = standingLeanMax * SubscribeBaseWeight

func scoreBand(score float64) int {
	return int(math.Floor(score / scoreBandWidth))
}

// signalWeight scores one raw signal at now, before trust-tier weighting is applied.
func signalWeight(s Signal, now time.Time) float64 {
	switch s.Kind {
	case SignalAuthor:
		return AuthorBaseWeight * standingLean(s.At, now)
	case SignalSubscribe:
		return SubscribeBaseWeight * standingLean(s.At, now)
	case SignalShare:
		return ShareBaseWeight * reactionDecay(s.At, now)
	case SignalSave:
		return SaveBaseWeight * reactionDecay(s.At, now)
	default:
		return 0
	}
}

func ageSince(at, now time.Time) time.Duration {
	age := now.Sub(at)
	if age < 0 {
		return 0
	}
	return age
}

// standingLean is a decreasing bonus multiplier, 1+leanMax at age zero approaching 1.0. A zero (unknown) anchor time degrades to the neutral 1.0, never an inflated bonus.
func standingLean(at, now time.Time) float64 {
	age := ageSince(at, now)
	halfLife := float64(standingLeanHalfLife)
	return 1.0 + standingLeanMax*halfLife/(halfLife+float64(age))
}

// reactionDecay is HN-gravity-style, 1.0 at age zero and decreasing. A zero (unknown) anchor time degrades to near-zero decay, same mechanism as standingLean.
func reactionDecay(at, now time.Time) float64 {
	age := ageSince(at, now)
	ageDays := age.Hours() / 24
	return math.Pow(reactionGravityOffsetDays/(ageDays+reactionGravityOffsetDays), reactionGravity)
}

// Follower is one raw signal a person contributes, tagged with its trust tier. A person may appear more than once; Rank reduces repeats to the strongest signal before scoring. SPEC <discovery>: one signal per person per source.
type Follower struct {
	DID     string
	Tier    Tier
	Network Network       // set only when Tier == TierWeak
	SelfApp SelfSourceApp // set only when Tier == TierSelf
	Signal  Signal
}

// WeakFollow is one adjacent-graph follow, before tier resolution against the user's strong follows.
type WeakFollow struct {
	DID     string
	Network Network
}

// ResolveFollowerTiers merges strong and weak follow sets into one per-DID trust assignment; a DID in both takes strong only. Where weakFollows lists the same DID under more than one network, the first occurrence wins, so callers order Bluesky before Tangled. SPEC <discovery>: a person in both tiers takes the higher, not the sum.
func ResolveFollowerTiers(strongDIDs []string, weakFollows []WeakFollow) map[string]Follower {
	out := make(map[string]Follower, len(strongDIDs)+len(weakFollows))
	for _, w := range weakFollows {
		if _, exists := out[w.DID]; exists {
			continue
		}
		out[w.DID] = Follower{DID: w.DID, Tier: TierWeak, Network: w.Network}
	}
	for _, did := range strongDIDs {
		out[did] = Follower{DID: did, Tier: TierStrong}
	}
	return out
}

// Candidate is one canonical source seen among the user's followed people's reader-network activity.
type Candidate struct {
	Key     string
	Kind    string // "rss" | "standardfeed"
	Title   string
	SiteURL string
	// Language is empty when unknown; only FilterByLanguage reads it. SPEC <discovery> Global/Trending ranking Language filter.
	Language discoverlang.Language
	// Followers may repeat a person; Rank reduces each to their strongest signal. Order doesn't matter.
	Followers []Follower
}

// followerDIDCap bounds Reason.FollowerDIDs to the frontend avatar stack size.
const followerDIDCap = 3

// Reason is the structured basis for a suggestion; the frontend formats it into English. SPEC <discovery>: every suggestion carries its reason.
type Reason struct {
	StrongCount int
	WeakCount   int
	// FollowerDIDs are up to followerDIDCap DIDs among the followers StrongCount/WeakCount count: strong-tier matches first, then weak-tier. Empty when SelfApp is credited.
	FollowerDIDs []string
	// TopFollowerDID is the strongest-signal contributor to credit: among strong-tier followers if any exist, else weak-tier; alphabetically-first DID breaks a kind tie.
	TopFollowerDID string
	// TopFollowerNetwork names TopFollowerDID's network when weak-tier; empty for a strong-tier top follower.
	TopFollowerNetwork Network
	// TopSignal is TopFollowerDID's signal kind, driving the reason's verb.
	TopSignal SignalKind
	// SelfApp is set when the top contributor is the user's own foreign-record signal; self outranks strong/weak at equal signal kind, so it takes priority over StrongCount/WeakCount in reason selection.
	SelfApp SelfSourceApp
}

// Suggestion is one ranked "For you" source card.
type Suggestion struct {
	Key     string
	Kind    string
	Title   string
	SiteURL string
	Reason  Reason
}

// Rank orders candidates by trust-weighted signal score descending, drops excluded keys and candidates with no followers, shuffles near-tie score bands by seed, and caps the result at max. now anchors every recency computation, keeping the engine pure. The shuffle runs before the cap, so which candidates survive a tied cutoff varies by seed too, not just their order among survivors. SPEC <discovery>: score(source) = sum of trust(p) x strongest signal(p, source).
// Saves never reach personal ranking: SPEC <saving-sharing> confines them to the anonymous trending batch, so they're dropped here before rankScored ever sees them. RankTrending scores raw saves via rankScored directly.
func Rank(candidates []Candidate, excluded map[string]struct{}, max int, seed int64, now time.Time) []Suggestion {
	return pageValues(RankPage(candidates, excluded, max, seed, now, nil))
}

// dropSaveFollowers strips save-signal followers before any tier/strongest-signal/score logic runs, so a save can never win a tier, become a TopSignal, or contribute score in personal ranking.
func dropSaveFollowers(followers []Follower) []Follower {
	out := make([]Follower, 0, len(followers))
	for _, f := range followers {
		if f.Signal.Kind == SignalSave {
			continue
		}
		out = append(out, f)
	}
	return out
}

// rankScored is Rank's scoring/ordering/reason-building core, shared with RankTrending (which calls it directly with unfiltered followers, since the trending path is the one place saves are allowed to score).
func rankScored(candidates []Candidate, excluded map[string]struct{}, max int, seed int64, now time.Time) []Suggestion {
	return pageValues(rankScoredPage(candidates, excluded, max, seed, now, nil))
}

// strongestPerFollower reduces repeated per-DID signals to the strongest, freshest breaking a kind tie. SPEC <discovery>: one signal per person per source, strongest wins.
func strongestPerFollower(followers []Follower) []Follower {
	best := make(map[string]Follower, len(followers))
	for _, f := range followers {
		cur, ok := best[f.DID]
		if !ok || StrongerSignal(f.Signal, cur.Signal) {
			best[f.DID] = f
		}
	}
	out := make([]Follower, 0, len(best))
	for _, f := range best {
		out = append(out, f)
	}
	return out
}

// StrongerSignal reports whether a beats b under author > subscribe > share > save, freshest breaking a kind tie. Exported so other pure modules reducing raw per-source signals reuse this comparator instead of redefining the ordering.
func StrongerSignal(a, b Signal) bool {
	if a.Kind != b.Kind {
		return a.Kind > b.Kind
	}
	return a.At.After(b.At)
}

// score applies trust-tier weights and the weak-tier cap after reducing to one signal per person: every strong follower counts in full, weak followers cap at roughly two strong subscribers' worth. SPEC <discovery> Weak-tier cap.
func score(followers []Follower, now time.Time) float64 {
	reduced := strongestPerFollower(followers)
	var self, strong, weakRaw float64
	for _, f := range reduced {
		w := signalWeight(f.Signal, now)
		switch f.Tier {
		case TierSelf:
			self += w * SelfWeight
		case TierStrong:
			strong += w * StrongWeight
		default:
			weakRaw += w * WeakWeight
		}
	}
	weakCap := WeakCapStrongEquivalents * SubscribeBaseWeight * StrongWeight
	if weakRaw > weakCap {
		weakRaw = weakCap
	}
	return self + strong + weakRaw
}

// reasonFor summarizes a candidate's reduced followers into its card reason, crediting a strong-tier contributor over weak whenever both exist, and within a tier the strongest signal kind present.
func reasonFor(followers []Follower) Reason {
	reduced := strongestPerFollower(followers)
	var self, strong, weak []Follower
	for _, f := range reduced {
		switch f.Tier {
		case TierSelf:
			self = append(self, f)
		case TierStrong:
			strong = append(strong, f)
		default:
			weak = append(weak, f)
		}
	}
	sortByTopSignalThenDID(self)
	sortByTopSignalThenDID(strong)
	sortByTopSignalThenDID(weak)

	var r Reason
	switch {
	case len(self) > 0:
		r.TopFollowerDID = self[0].DID
		r.TopSignal = self[0].Signal.Kind
		r.SelfApp = self[0].SelfApp
	case len(strong) > 0:
		r.TopFollowerDID = strong[0].DID
		r.TopSignal = strong[0].Signal.Kind
	case len(weak) > 0:
		r.TopFollowerDID = weak[0].DID
		r.TopFollowerNetwork = weak[0].Network
		r.TopSignal = weak[0].Signal.Kind
	}
	r.StrongCount = countBySignal(strong, r.TopSignal)
	r.WeakCount = countBySignal(weak, r.TopSignal)
	if len(self) == 0 {
		dids := append(followerDIDsBySignal(strong, r.TopSignal), followerDIDsBySignal(weak, r.TopSignal)...)
		if len(dids) > followerDIDCap {
			dids = dids[:followerDIDCap]
		}
		r.FollowerDIDs = dids
	}
	return r
}

// countBySignal counts followers carrying exactly the credited top signal, so a weaker contributor never inflates the count the frontend renders with that signal's verb.
func countBySignal(fs []Follower, topSignal SignalKind) int {
	n := 0
	for _, f := range fs {
		if f.Signal.Kind == topSignal {
			n++
		}
	}
	return n
}

// followerDIDsBySignal returns the DIDs of fs carrying exactly topSignal, preserving fs's order.
func followerDIDsBySignal(fs []Follower, topSignal SignalKind) []string {
	var dids []string
	for _, f := range fs {
		if f.Signal.Kind == topSignal {
			dids = append(dids, f.DID)
		}
	}
	return dids
}

// sortByTopSignalThenDID orders strongest-signal-first, alphabetically-first DID breaking a kind tie, deterministic regardless of caller input order.
func sortByTopSignalThenDID(fs []Follower) {
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].Signal.Kind != fs[j].Signal.Kind {
			return fs[i].Signal.Kind > fs[j].Signal.Kind
		}
		return fs[i].DID < fs[j].DID
	})
}

// shuffleKey maps (seed, key) to a per-key hash for ordering within a tied score band, not a positional Fisher-Yates, so the result never depends on caller iteration order.
func shuffleKey(seed int64, key string) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(seed))
	_, _ = h.Write(buf[:])
	_, _ = h.Write([]byte(key))
	return h.Sum64()
}
