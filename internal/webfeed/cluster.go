package webfeed

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

// cluster is one candidate list: links that repeat under one container.
type cluster struct {
	container *html.Node
	hint      string
	anchors   []*anchor
	entries   []*entry
	template  string

	score, tmplShare, headShare, dateFrac, titleShape, navShare, asideShare float64
	taxShare, socialShare, pagShare, externalShare, slugShare, underShare   float64
	utilShare                                                               float64
	decoy                                                                   bool
}

// entry is one post-like item inside a cluster.
type entry struct {
	URL         string
	Title       string
	Date        time.Time
	TitleSource string
	titleTag    string
	node        *html.Node
	text        string
	inNav       bool
	inAside     bool
	seeAll      bool
}

// clusters groups the page's anchors into candidate lists, unscored and in document order.
func (p *pageDoc) clusters() []*cluster {
	byTag := map[string][]*anchor{}
	var tagOrder []string
	for _, a := range p.anchors {
		// The link's own classes separate roles that share a tag path, such as a card's image link and its category link.
		k := a.tagPath + "|" + nodeSig(a.node, true)
		if _, ok := byTag[k]; !ok {
			tagOrder = append(tagOrder, k)
		}
		byTag[k] = append(byTag[k], a)
	}
	var groups [][]*anchor
	for _, tp := range tagOrder {
		groups = append(groups, splitGroup(byTag[tp])...)
	}

	var out []*cluster
	for _, g := range groups {
		c := &cluster{anchors: g, container: p.groupContainer(g)}
		if m := findMergeTarget(out, c); m != nil {
			m.anchors = append(m.anchors, c.anchors...)
			continue
		}
		out = append(out, c)
	}
	var kept []*cluster
	for _, c := range out {
		slices.SortStableFunc(c.anchors, func(a, b *anchor) int { return p.order[a.node] - p.order[b.node] })
		p.buildEntries(c)
		if len(c.entries) == 0 {
			continue
		}
		if len(c.entries) == 1 && !p.singletonWorthKeeping(c.entries[0]) {
			continue
		}
		c.hint = containerHint(c.container)
		kept = append(kept, c)
	}
	return kept
}

// splitGroup separates anchors that share a tag path into distinct lists. It splits only at a level whose nodes hold
// several posts each (sub-containers) and differ in classes; entries of one list may differ in classes freely.
func splitGroup(g []*anchor) [][]*anchor {
	if len(g) < 2 {
		return [][]*anchor{g}
	}
	d := lcaDepth(g)
	if d+1 >= len(g[0].chain) {
		return [][]*anchor{g}
	}
	bySig := map[string][]*anchor{}
	var sigs []string
	containerLevel := false
	keysUnder := map[*html.Node]map[string]bool{}
	for _, a := range g {
		n := a.chain[d+1]
		if keysUnder[n] == nil {
			keysUnder[n] = map[string]bool{}
		}
		keysUnder[n][a.key] = true
		containerLevel = containerLevel || len(keysUnder[n]) > 1
		sig := nodeSig(n, true)
		if _, ok := bySig[sig]; !ok {
			sigs = append(sigs, sig)
		}
		bySig[sig] = append(bySig[sig], a)
	}
	if !containerLevel || len(sigs) == 1 {
		return [][]*anchor{g}
	}
	var out [][]*anchor
	for _, sig := range sigs {
		out = append(out, splitGroup(bySig[sig])...)
	}
	return out
}

func lcaDepth(g []*anchor) int {
	d := 0
	for d < len(g[0].chain) {
		n := g[0].chain[d]
		for _, a := range g[1:] {
			if d >= len(a.chain) || a.chain[d] != n {
				return d - 1
			}
		}
		d++
	}
	return d - 1
}

// groupContainer is the list element: the common ancestor of a multi-link group, or the parent of a lone link's own entry.
func (p *pageDoc) groupContainer(g []*anchor) *html.Node {
	distinct := map[string]bool{}
	for _, a := range g {
		distinct[a.key] = true
	}
	if len(distinct) > 1 {
		return g[0].chain[lcaDepth(g)]
	}
	e := p.soleOwner(g[0])
	if e.Parent != nil && e.Parent.Type == html.ElementNode {
		return e.Parent
	}
	return e
}

// soleOwner is the highest ancestor of a that links nowhere but a's URL.
func (p *pageDoc) soleOwner(a *anchor) *html.Node {
	if p.mixed == nil {
		p.mixed = map[*html.Node]bool{}
		keyOf := map[*html.Node]string{}
		for _, o := range p.anchors {
			for _, n := range o.chain {
				if k, ok := keyOf[n]; !ok {
					keyOf[n] = o.key
				} else if k != o.key {
					p.mixed[n] = true
				}
			}
		}
	}
	best := a.node
	for i := len(a.chain) - 2; i >= 0; i-- {
		n := a.chain[i]
		if n.Data == "body" || n.Data == "html" || n.Data == "main" || p.mixed[n] {
			break
		}
		best = n
	}
	return best
}

// findMergeTarget joins link groups that point at the same posts from one container, like a card's image and title links.
// Disjoint groups stay apart even with one URL shape: a card's category links often share the posts' shape.
func findMergeTarget(out []*cluster, c *cluster) *cluster {
	for _, m := range out {
		if m.container == c.container && urlOverlap(m.anchors, c.anchors) >= 0.5 {
			return m
		}
	}
	return nil
}

func urlOverlap(a, b []*anchor) float64 {
	as, bs := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		as[x.key] = true
	}
	for _, x := range b {
		bs[x.key] = true
	}
	shared := 0
	for k := range bs {
		if as[k] {
			shared++
		}
	}
	return float64(shared) / float64(min(len(as), len(bs)))
}

// buildEntries assigns each distinct URL its entry element: the highest ancestor below the container holding no other URL of the cluster.
func (p *pageDoc) buildEntries(c *cluster) {
	keysUnder := map[*html.Node]map[string]bool{}
	for _, a := range c.anchors {
		for i := len(a.chain) - 1; i >= 0 && a.chain[i] != c.container; i-- {
			n := a.chain[i]
			if keysUnder[n] == nil {
				keysUnder[n] = map[string]bool{}
			}
			keysUnder[n][a.key] = true
		}
	}
	seen := map[string]*entry{}
	for _, a := range c.anchors {
		if seen[a.key] != nil {
			continue
		}
		owner := a.node
		for i := len(a.chain) - 1; i >= 0 && a.chain[i] != c.container; i-- {
			if len(keysUnder[a.chain[i]]) > 1 {
				break
			}
			owner = a.chain[i]
		}
		if len(c.anchors) == 1 || c.container == owner {
			owner = p.soleOwner(a)
		}
		e := &entry{URL: a.url, node: owner}
		e.inNav, e.inAside = navContext(a.node)
		seen[a.key] = e
		c.entries = append(c.entries, e)
	}
	slices.SortStableFunc(c.entries, func(a, b *entry) int { return p.order[a.node] - p.order[b.node] })
	c.template = entryTemplate(c.entries)
	p.fillEntries(c)
	c.entries = slices.DeleteFunc(c.entries, func(e *entry) bool { return e.seeAll })
	if len(c.entries) > 0 {
		c.template = entryTemplate(c.entries)
	}
}

// entryTemplate is the list's URL shape: its most common template when that covers 80%, else a generalized pattern when a category in the path splits it.
func entryTemplate(entries []*entry) string {
	counts := map[string]int{}
	buckets := map[string][]string{}
	best, bestN := "", 0
	for _, e := range entries {
		t := URLTemplate(e.URL)
		counts[t]++
		if counts[t] > bestN {
			best, bestN = t, counts[t]
		}
		b := strings.Count(t, "/")
		key := t[:strings.Index(t, "/")+1] + strconv.Itoa(b)
		buckets[key] = append(buckets[key], t)
	}
	n := len(entries)
	if bestN*10 >= n*8 {
		return best
	}
	for _, ts := range buckets {
		if len(ts)*10 < n*8 {
			continue
		}
		if g := generalizeTemplates(ts); g != "" {
			return g
		}
	}
	return best
}

// singletonWorthKeeping keeps lone links that look like a featured post, the hero block of a split stream.
func (p *pageDoc) singletonWorthKeeping(e *entry) bool {
	if e.inNav {
		return false
	}
	if !postLikePath(e.URL) {
		return false
	}
	return e.TitleSource == "heading" || len(strings.Fields(e.Title)) >= 4
}

// containerHint renders the container and two ancestors as "tag.class<tag.class<tag".
func containerHint(n *html.Node) string {
	var parts []string
	for x := n; x != nil && x.Type == html.ElementNode && len(parts) < 3; x = x.Parent {
		if x.Data == "html" {
			break
		}
		parts = append(parts, nodeSig(x, false))
		if x.Data == "body" {
			break
		}
	}
	return strings.Join(parts, "<")
}

// hintSkeleton drops class names so a rehashed stylesheet still matches by structure.
func hintSkeleton(hint string) string {
	parts := strings.Split(hint, "<")
	for i, p := range parts {
		parts[i], _, _ = strings.Cut(p, ".")
	}
	return strings.Join(parts, "<")
}

// nodeSig is tag plus classes; stable normalizes per-item and build-hash classes away.
func nodeSig(n *html.Node, stable bool) string {
	sig := n.Data
	for _, c := range classes(n) {
		if stable {
			if c = stableClass(c); c == "" {
				continue
			}
		}
		sig += "." + c
	}
	return sig
}

// stableClass cuts a digit-bearing suffix (post-123, Card__x8Ab2, css-1x2y3z): CMS item ids and build hashes, not structure.
func stableClass(c string) string {
	if !strings.ContainsFunc(c, unicode.IsDigit) {
		return c
	}
	i := strings.LastIndexAny(c, "-_")
	if i <= 0 {
		return ""
	}
	return stableClass(strings.TrimRight(c[:i], "-_"))
}

// navContext reports whether a link sits in site navigation (nav, page header/footer) or a sidebar.
func navContext(n *html.Node) (inNav, inAside bool) {
	sectioned := false
	for x := n.Parent; x != nil && x.Type == html.ElementNode; x = x.Parent {
		switch x.Data {
		case "nav":
			return true, inAside
		case "article", "section", "main":
			sectioned = true
		case "aside":
			inAside = true
		case "header", "footer":
			if !sectioned {
				return true, inAside
			}
		}
		switch strings.ToLower(attr(x, "role")) {
		case "navigation", "banner", "contentinfo", "menubar", "menu":
			return true, inAside
		case "complementary":
			inAside = true
		}
		if !sectioned && x.Data != "body" {
			for _, tok := range nameTokens(x) {
				switch tok {
				case "nav", "navbar", "navigation", "menu", "footer", "breadcrumb", "breadcrumbs", "megamenu":
					return true, inAside
				case "sidebar":
					inAside = true
				}
			}
		}
	}
	return false, inAside
}

// nameTokens splits class and id values into lowercase words.
func nameTokens(n *html.Node) []string {
	return strings.FieldsFunc(strings.ToLower(attr(n, "class")+" "+attr(n, "id")), func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == ':' || r == '/' || r == '\t' || r == '\n'
	})
}
