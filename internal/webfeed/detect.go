package webfeed

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	"morgenblau/internal/postkey"
)

// PollVerdict is the detector's call on one poll: ok publishes, suspect repairs, broken waits.
type PollVerdict string

const (
	PollOK      PollVerdict = "ok"
	PollSuspect PollVerdict = "suspect"
	PollBroken  PollVerdict = "broken"
)

// Reason is a coded cause behind a poll verdict or a setup's health; the server words it for readers.
type Reason string

// Detector reasons.
const (
	ReasonUnreachable   Reason = "unreachable"
	ReasonRobots        Reason = "robots"
	ReasonHTTPStatus    Reason = "http_status"
	ReasonNotHTML       Reason = "not_html"
	ReasonChallenge     Reason = "challenge"
	ReasonErrorPage     Reason = "error_page"
	ReasonConsentWall   Reason = "consent_wall"
	ReasonSPAShell      Reason = "spa_shell"
	ReasonEmpty         Reason = "empty"
	ReasonCountRatio    Reason = "count_ratio"
	ReasonContinuity    Reason = "continuity"
	ReasonTemplateShare Reason = "template_share"
	ReasonTitleShape    Reason = "title_shape"
	ReasonDupTitles     Reason = "dup_titles"
)

// Setup reasons, beside the detector's title_shape, dup_titles, template_share and robots.
const (
	// ReasonCount is a setup that found fewer than three posts.
	ReasonCount Reason = "count"
	// ReasonUnsure is a pick the heuristic doubted and no judge settled.
	ReasonUnsure Reason = "unsure"
	// ReasonJudgeNone is a judge majority saying no list on the page is a post stream.
	ReasonJudgeNone Reason = "judge_none"
	// ReasonBlocked is a listing that refused us with no sitemap to fall back on.
	ReasonBlocked Reason = "blocked"
)

// GoodPoll is one ok poll in the detector's history.
type GoodPoll struct {
	URLs        []string
	Titles      []string
	URLTemplate string
	TitleShape  float64
}

// DominantTemplate is the URL shape most items share, counted under the first recipe pattern each one fits.
func DominantTemplate(items []Item, templates []string) string {
	counts := map[string]int{}
	best := ""
	for _, it := range items {
		t := URLTemplate(it.URL)
		for _, p := range templates {
			if matchTemplate(p, t) {
				t = p
				break
			}
		}
		counts[t]++
		if counts[t] > counts[best] {
			best = t
		}
	}
	return best
}

// responseTier0 checks what the fetch alone shows; blocked means the site refused us rather than broke.
func responseTier0(r *response, wantHTML bool) (reason Reason, blocked bool) {
	if reason, blocked := r.refusal(); reason != "" {
		return reason, blocked
	}
	if wantHTML && !r.isHTML() {
		return ReasonNotHTML, false
	}
	return "", false
}

var reErrorTitle = regexp.MustCompile(`(?i)^\W*(error\W*)?(404|410|500|502|503)\b`)

// errorPhrases name a soft error page; they are matched on the title and first heading only, where a listing never says them.
var errorPhrases = []string{
	"page not found", "not found", "404 error", "error 404", "internal server error", "server error", "service unavailable",
	"bad gateway", "something went wrong", "an error occurred", "page unavailable", "temporarily unavailable", "under maintenance",
	"seite nicht gefunden", "seite wurde nicht gefunden", "page introuvable", "página no encontrada", "pagina non trovata",
	"pagina niet gevonden", "ページが見つかりません",
}

// consentWallPhrases describe a gate in front of the page, not a banner beside it; a match alone is enough.
var consentWallPhrases = []string{
	"before you continue", "bevor sie fortfahren", "bevor du fortfährst", "avant de continuer", "antes de continuar", "prima di continuare",
	"accept cookies to continue", "accept all cookies to continue", "accept cookies to view", "consent to continue",
	"to continue, please accept", "you must accept cookies", "you need to accept cookies", "this site requires cookies",
	"cookie wall", "cookiewall", "zustimmung erforderlich", "werbefrei mit pur", "pur-abo",
}

// consentMentions only count on a page with no posts, where a banner is all there is.
var consentMentions = []string{"cookie", "consent", "privacy settings", "privacy choices", "datenschutz", "einwilligung", "gdpr"}

// spaMarkers are empty mount points that client-rendered sites fill with JavaScript.
var spaMarkers = [][]byte{
	[]byte(`id="root"`), []byte(`id="__next"`), []byte(`id="app"`), []byte(`id="__nuxt"`), []byte(`id="svelte"`),
	[]byte(`<app-root`), []byte(`data-reactroot`), []byte(`ng-version`),
}

const (
	tier0TextBytes = 3000
	spaShellText   = 400
)

func containsAny(s string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func visibleText(p *pageDoc) string {
	body := p.doc.Find("body")
	if body.Length() == 0 {
		return ""
	}
	return nodeText(body.Nodes[0])
}

func textHead(s string) string {
	return strings.ToLower(s[:min(len(s), tier0TextBytes)])
}

// pageTier0 checks what the parsed page shows before any extraction.
func pageTier0(p *pageDoc) Reason {
	title := strings.ToLower(p.title)
	h1 := strings.ToLower(collapse(p.doc.Find("h1").First().Text()))
	for _, s := range []string{title, h1} {
		if reErrorTitle.MatchString(s) || containsAny(s, errorPhrases) {
			return ReasonErrorPage
		}
	}
	if containsAny(title, consentWallPhrases) || containsAny(textHead(visibleText(p)), consentWallPhrases) {
		return ReasonConsentWall
	}
	return ""
}

// emptyTier0 explains an extraction with no items when the page itself is the problem.
func emptyTier0(p *pageDoc, body []byte) Reason {
	text := visibleText(p)
	if len(text) < spaShellText {
		lower := bytes.ToLower(body)
		for _, m := range spaMarkers {
			if bytes.Contains(lower, m) {
				return ReasonSPAShell
			}
		}
	}
	if containsAny(strings.ToLower(p.title), consentMentions) || containsAny(textHead(text), consentMentions) {
		return ReasonConsentWall
	}
	return ""
}

// drift applies the suspect rules against the good-poll history, newest first; the first match wins.
func drift(items []Item, dominant string, history []GoodPoll) Reason {
	n := len(items)
	if n == 0 {
		return ReasonEmpty
	}
	sig := ComputeSignals(items, nil)
	if len(history) > 0 {
		if med := medianCount(history); med > 0 {
			if ratio := float64(n) / med; ratio < 0.5 || ratio > 3 {
				return ReasonCountRatio
			}
		}
		last := history[0]
		if len(last.URLs) > 0 {
			known := map[string]bool{}
			for _, u := range last.URLs {
				known[postkey.URL(u)] = true
			}
			current := map[string]bool{}
			novel := 0
			for _, it := range items {
				k := postkey.URL(it.URL)
				current[k] = true
				if !known[k] {
					novel++
				}
			}
			present := 0
			for k := range known {
				if current[k] {
					present++
				}
			}
			if float64(present)/float64(len(known)) < 0.5 && float64(novel)/float64(n) > 0.5 {
				return ReasonContinuity
			}
		}
		if hist := last.URLTemplate; hist != "" {
			matching := 0
			for _, it := range items {
				if matchTemplate(hist, URLTemplate(it.URL)) {
					matching++
				}
			}
			if float64(matching)/float64(n) < 0.5 && dominant != hist {
				return ReasonTemplateShare
			}
		}
		if sig.TitleShape < 0.7 && last.TitleShape >= 0.8 {
			return ReasonTitleShape
		}
	}
	if sig.DupTitles > 0.5 {
		return ReasonDupTitles
	}
	return ""
}

func medianCount(history []GoodPoll) float64 {
	counts := make([]int, len(history))
	for i, h := range history {
		counts[i] = len(h.URLs)
	}
	slices.Sort(counts)
	mid := len(counts) / 2
	if len(counts)%2 == 1 {
		return float64(counts[mid])
	}
	return float64(counts[mid-1]+counts[mid]) / 2
}
