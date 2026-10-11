package feedfinder

import "testing"

func TestResolve_ClientRenderedListingAcceptsAGuessedFeed(t *testing.T) {
	for name, body := range map[string]string{
		"empty shell": `<div id="root"></div><script src="/app.js"></script>`,
		"shell with site chrome": `<header><a href="https://blog.example.com/">Home</a></header>
<nav><a href="https://blog.example.com/about">About</a><a href="https://blog.example.com/archive">Archive</a></nav>
<div id="root"></div>
<footer><a href="https://blog.example.com/privacy">Privacy</a><a href="https://blog.example.com/imprint">Imprint</a></footer>`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := newSiteServer(t, map[string]page{
				"/blog":     htmlPage(`<html><head><title>Example Publication</title></head><body>` + body + `</body></html>`),
				"/feed.xml": rssPage("Example Publication", "https://blog.example.com/posts/first", "https://blog.example.com/posts/second"),
			})

			got := resolveOne(t, srv.URL+"/blog")
			if got.FeedURL != srv.URL+"/feed.xml" {
				t.Errorf("FeedURL = %q, want the guessed feed", got.FeedURL)
			}
		})
	}
}
