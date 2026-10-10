package webfeed

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// URLTemplate returns the URL shape a stream is keyed on: lowercase host without "www.", all-digit path segments as #, the last segment as *.
func URLTemplate(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	var segs []string
	for s := range strings.SplitSeq(u.Path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	// A directory's default document is the directory: /post/index.html is /post/.
	if n := len(segs); n > 0 && defaultDocuments[strings.ToLower(segs[n-1])] {
		segs = segs[:n-1]
	}
	if len(segs) == 0 {
		return host + "/"
	}
	for i, s := range segs {
		switch {
		case i == len(segs)-1:
			segs[i] = "*"
		case allDigits(s):
			segs[i] = "#"
		}
	}
	return host + "/" + strings.Join(segs, "/")
}

// depthKey groups templates by host and path depth, the templates generalizeTemplates may merge.
func depthKey(tmpl string) string {
	return tmpl[:strings.Index(tmpl, "/")+1] + strconv.Itoa(strings.Count(tmpl, "/"))
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// templateFirstSegment is the first path segment of a template, "" when the stream sits at the root.
func templateFirstSegment(tmpl string) string {
	_, path, ok := strings.Cut(tmpl, "/")
	if !ok {
		return ""
	}
	first, _, more := strings.Cut(path, "/")
	if !more {
		return ""
	}
	return first
}

// pathFirstSegment is the first segment of a URL's path.
func pathFirstSegment(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	return first
}

var defaultDocuments = map[string]bool{"index.html": true, "index.htm": true, "index.php": true, "default.aspx": true, "default.htm": true}

// lastSegment is the URL's final path segment, skipping a default document such as index.html.
func lastSegment(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	var segs []string
	for s := range strings.SplitSeq(u.Path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if n := len(segs); n > 0 && defaultDocuments[strings.ToLower(segs[n-1])] {
		segs = segs[:n-1]
	}
	if len(segs) == 0 {
		return "", u.RawQuery != ""
	}
	return segs[len(segs)-1], true
}

// generalizeTemplates merges one list's same-depth templates whose middle segments vary (a category in the path) into one pattern.
// It returns "" unless they share host and depth and keep a literal segment, so unrelated pages never merge.
func generalizeTemplates(ts []string) string {
	distinct := map[string]bool{}
	for _, t := range ts {
		distinct[t] = true
	}
	if len(distinct) < 2 {
		return ""
	}
	var host string
	var segs [][]string
	for t := range distinct {
		h, path, _ := strings.Cut(t, "/")
		if host != "" && h != host {
			return ""
		}
		host = h
		s := strings.Split(path, "/")
		if len(segs) > 0 && len(s) != len(segs[0]) {
			return ""
		}
		segs = append(segs, s)
	}
	out := slices.Clone(segs[0])
	literal := false
	for i := range out {
		for _, s := range segs[1:] {
			if s[i] != out[i] {
				out[i] = "*"
				break
			}
		}
		if i < len(out)-1 && out[i] != "*" && out[i] != "#" {
			literal = true
		}
	}
	if !literal {
		return ""
	}
	return host + "/" + strings.Join(out, "/")
}

// matchTemplate reports whether a URL's template fits a stream's pattern, where * in the pattern stands for any one segment.
func matchTemplate(pattern, tmpl string) bool {
	if pattern == tmpl {
		return true
	}
	ph, pp, _ := strings.Cut(pattern, "/")
	th, tp, _ := strings.Cut(tmpl, "/")
	if ph != th {
		return false
	}
	ps, ts := strings.Split(pp, "/"), strings.Split(tp, "/")
	if len(ps) != len(ts) {
		return false
	}
	for i := range ps {
		if ps[i] != "*" && ps[i] != ts[i] {
			return false
		}
	}
	return true
}

func matchAny(patterns []string, tmpl string) bool {
	for _, p := range patterns {
		if matchTemplate(p, tmpl) {
			return true
		}
	}
	return false
}
