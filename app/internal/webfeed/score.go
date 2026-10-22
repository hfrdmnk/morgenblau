package webfeed

import (
	"math"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"

	"morgenblau/internal/postkey"
)

// maxCandidates is how many lists the judge sees.
const maxCandidates = 6

// candidates scores every cluster on the page, best first.
func (p *pageDoc) candidates() []*cluster {
	cs := p.clusters()
	for _, c := range cs {
		p.score(c)
	}
	slices.SortStableFunc(cs, func(a, b *cluster) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		}
		return 0
	})
	// Responsive pages render one list twice (desktop and mobile); the copy would only crowd the judge's six slots.
	seen := map[string]bool{}
	return slices.DeleteFunc(cs, func(c *cluster) bool {
		var keys []string
		for _, e := range c.entries {
			keys = append(keys, postkey.URL(e.URL))
		}
		k := strings.Join(keys, " ")
		dup := seen[k]
		seen[k] = true
		return dup
	})
}

// score adds points: size grows with log2(n), each share-based feature moves the score by up to 1–5 points,
// so the judge's margin threshold of 2 means "one decisive feature apart".
func (p *pageDoc) score(c *cluster) {
	n := float64(len(c.entries))
	var tmpl, head, dated, shape, nav, aside, tax, social, pag, ext, slug, under, util float64
	listingPath := strings.TrimSuffix(p.url.Path, "/")
	for _, e := range c.entries {
		if matchTemplate(c.template, URLTemplate(e.URL)) {
			tmpl++
		}
		if e.TitleSource == TitleHeading || e.TitleSource == TitleClass {
			head++
		}
		if !e.Date.IsZero() {
			dated++
		}
		if goodTitleShape(e.Title) {
			shape++
		}
		if e.inNav {
			nav++
		}
		if e.inAside {
			aside++
		}
		u, err := url.Parse(e.URL)
		if err != nil {
			continue
		}
		switch {
		case isSocialHost(u.Host):
			social++
		case !sameSite(u.Host, p.url.Host):
			ext++
		}
		if isTaxonomy(u) {
			tax++
		}
		if isPagination(u, e.Title) {
			pag++
		}
		if isUtility(u, e.Title) {
			util++
		}
		if postLikePath(e.URL) {
			slug++
		}
		if listingPath != "" && strings.HasPrefix(u.Path, listingPath+"/") && len(strings.TrimSuffix(u.Path, "/")) > len(listingPath)+1 {
			under++
		}
	}
	c.tmplShare, c.headShare, c.dateFrac, c.titleShape = tmpl/n, head/n, dated/n, shape/n
	c.navShare, c.asideShare, c.taxShare, c.socialShare = nav/n, aside/n, tax/n, social/n
	c.pagShare, c.externalShare, c.slugShare, c.underShare = pag/n, ext/n, slug/n, under/n
	c.utilShare = util / n

	s := 1.2 * math.Log2(min(n, 64))
	s += 2*c.tmplShare + 1.5*c.headShare + 2*c.dateFrac + 2*c.titleShape + 1.5*c.slugShare + c.underShare
	s -= 4*c.navShare + 1.5*c.asideShare + 4*c.taxShare + 5*c.socialShare + 3*c.pagShare + 2*c.externalShare + 4*c.utilShare
	c.decoy = isDecoyLabel(sectionLabel(c.container))
	if c.decoy {
		s -= 2
	}
	c.score = s
}

// confidence rates the top candidate in [0,1] and returns its score margin over the best candidate its shape does not
// cover: same-shape blocks join the pick anyway, so their closeness is not doubt.
func confidence(cs []*cluster) (conf, margin float64) {
	if len(cs) == 0 {
		return 0, 0
	}
	top := cs[0]
	size := min(1, float64(len(top.entries))/5)
	evidence := max(top.dateFrac, top.slugShare, top.headShare)
	conf = 0.25*top.tmplShare + 0.2*top.titleShape + 0.15*size + 0.25*evidence + 0.15*(1-top.navShare)
	conf -= 0.5 * (top.taxShare + top.socialShare + top.pagShare + top.externalShare + top.utilShare)
	conf = math.Max(0, math.Min(1, conf))
	margin = top.score
	for _, c := range cs[1:] {
		if !matchTemplate(top.template, c.template) {
			margin = top.score - c.score
			break
		}
	}
	return conf, margin
}

// heuristicPick is the top candidate plus every post-like candidate whose URL shape the top's shape covers.
func heuristicPick(cs []*cluster, listingURL string) []int {
	if len(cs) == 0 {
		return nil
	}
	pick := []int{0}
	for i, c := range cs[1:] {
		postLike := c.titleShape >= 0.5 && max(c.slugShare, c.dateFrac, c.headShare) >= 0.5
		if matchTemplate(cs[0].template, c.template) && c.navShare < 0.5 && c.asideShare < 0.5 && !c.decoy && postLike {
			pick = append(pick, i+1)
		}
	}
	return sectionGuard(cs, pick, listingURL)
}

// sectionGuard drops extra picks outside the listing's first path segment. The primary pick's segment also passes,
// so root listings and /blog pages whose posts live under /posts keep their extra blocks.
func sectionGuard(cs []*cluster, pick []int, listingURL string) []int {
	if len(pick) < 2 {
		return pick
	}
	listingSeg := pathFirstSegment(listingURL)
	primarySeg := templateFirstSegment(cs[pick[0]].template)
	out := []int{pick[0]}
	for _, i := range pick[1:] {
		seg := templateFirstSegment(cs[i].template)
		if seg == listingSeg || seg == primarySeg {
			out = append(out, i)
		}
	}
	return out
}

// goodTitleShape is the detector's title_shape test: 2–30 words, not all caps, not a bare number or date.
func goodTitleShape(t string) bool {
	w := wordCount(t)
	if w < 2 || w > 30 {
		return false
	}
	if isDateLike(t) || !hasLetter(t) {
		return false
	}
	letters, cased, upper := 0, 0, 0
	for _, r := range t {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.IsUpper(r) != unicode.IsLower(r) {
			cased++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	// Latin acronyms in a Japanese title are not shouting; only judge case where cased letters dominate.
	return cased < 4 || cased*2 < letters || upper < cased
}

var taxonomySegments = map[string]bool{
	"tag": true, "tags": true, "category": true, "categories": true, "author": true, "authors": true, "topic": true, "topics": true,
	"label": true, "labels": true, "archive": true, "archives": true, "user": true, "users": true, "profile": true, "kategorie": true,
	"schlagwort": true, "themen": true, "thema": true, "categorie": true, "categoria": true, "etiqueta": true, "autor": true, "auteur": true,
}

func isTaxonomy(u *url.URL) bool {
	for s := range strings.SplitSeq(strings.ToLower(u.Path), "/") {
		if taxonomySegments[s] {
			return true
		}
	}
	q := u.Query()
	return q.Has("tag") || q.Has("category") || q.Has("cat") || q.Has("author") || q.Has("topic")
}

var rePageSegment = regexp.MustCompile(`(?i)/(page|p|seite|pagina|página)/\d+/?$`)

var paginationWords = map[string]bool{
	"next": true, "previous": true, "prev": true, "older": true, "newer": true, "older posts": true, "newer posts": true,
	"next page": true, "previous page": true, "weiter": true, "zurück": true, "suivant": true, "précédent": true, "siguiente": true,
	"anterior": true, "次へ": true, "前へ": true, "last": true, "first": true, "»": true, "«": true, "›": true, "‹": true,
}

func isPagination(u *url.URL, title string) bool {
	if rePageSegment.MatchString(u.Path) {
		return true
	}
	q := u.Query()
	for _, k := range []string{"page", "p", "pg", "offset", "start", "cursor"} {
		if v := q.Get(k); v != "" && allDigits(v) {
			return true
		}
	}
	t := strings.ToLower(strings.Trim(title, " .…→←"))
	return paginationWords[t] || allDigits(t)
}

// postLikePath reports a URL that reads like a single item: a multi-word slug, a long token or a numeric id.
func postLikePath(raw string) bool {
	last, ok := lastSegment(raw)
	if last == "" {
		return ok
	}
	last = strings.TrimSuffix(strings.TrimSuffix(last, ".html"), ".htm")
	if allDigits(last) {
		return len(last) >= 2
	}
	if strings.Count(last, "-")+strings.Count(last, "_") >= 2 || len(last) >= 16 {
		return true
	}
	return strings.ContainsFunc(last, unicode.IsDigit) && len(last) >= 6
}

var socialHosts = []string{
	"twitter.com", "x.com", "facebook.com", "fb.com", "instagram.com", "linkedin.com", "youtube.com", "youtu.be", "tiktok.com",
	"github.com", "bsky.app", "threads.net", "mastodon.social", "reddit.com", "discord.gg", "discord.com", "t.me", "telegram.me",
	"pinterest.com", "whatsapp.com", "wa.me", "news.ycombinator.com", "patreon.com", "twitch.tv", "vimeo.com",
}

func isSocialHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	for _, s := range socialHosts {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

// sameSite compares registrable domains, enough to tell a blog subdomain from another site.
func sameSite(a, b string) bool {
	return siteOf(a) == siteOf(b)
}

// siteOf is the registrable domain, the host itself when the public suffix list cannot name one (an IP, localhost).
func siteOf(host string) string {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if site, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return site
	}
	return host
}

var decoyWords = []string{
	"popular", "most read", "most viewed", "trending", "related", "recommended", "you may also like", "you might also like",
	"read next", "more from", "editor's pick", "editors' pick", "top stories", "beliebt", "meistgelesen", "empfohlen",
	"ähnliche", "populaire", "les plus lus", "populares", "más leídos", "lo más", "人気", "おすすめ", "関連",
}

func isDecoyLabel(label string) bool {
	l := strings.ToLower(label)
	for _, w := range decoyWords {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// sectionLabel is the heading that introduces a list: a heading just before the container or an ancestor, at most three levels up.
func sectionLabel(n *html.Node) string {
	for x, level := n, 0; x != nil && x.Type == html.ElementNode && level < 3; x, level = x.Parent, level+1 {
		if x.Data == "main" || x.Data == "body" {
			break
		}
		for sib, k := x.PrevSibling, 0; sib != nil && k < 3; sib = sib.PrevSibling {
			if sib.Type != html.ElementNode {
				continue
			}
			k++
			if isHeading(sib) {
				return nodeText(sib)
			}
			if hs := descendants(sib, isHeading); len(hs) == 1 && len(nodeText(sib)) < 80 {
				return nodeText(hs[0])
			}
		}
	}
	return ""
}

// utilityPages are the site-furniture pages every site links: legal, account, company and help pages.
var utilityPages = map[string]bool{
	"privacy": true, "privacy-policy": true, "privacy policy": true, "terms": true, "terms-of-use": true, "terms of use": true,
	"terms-of-service": true, "terms of service": true, "tos": true, "legal": true, "imprint": true, "impressum": true,
	"datenschutz": true, "datenschutzerklärung": true, "agb": true, "cookies": true, "cookie-policy": true, "cookie policy": true,
	"code-of-conduct": true, "code of conduct": true, "coc": true, "accessibility": true, "contact": true, "contact us": true,
	"about": true, "about us": true, "careers": true, "jobs": true, "pricing": true, "login": true, "log in": true, "sign in": true,
	"signup": true, "sign-up": true, "sign up": true, "security": true, "status": true, "sitemap": true, "help": true, "faq": true,
	"mentions-legales": true, "mentions légales": true, "aviso-legal": true, "aviso legal": true, "kontakt": true,
}

func isUtility(u *url.URL, title string) bool {
	last, _ := lastSegment(u.String())
	return utilityPages[strings.ToLower(last)] || utilityPages[strings.ToLower(strings.TrimSpace(title))]
}
