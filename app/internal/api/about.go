package api

import "net/http"

const aboutBody = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>About Morgenblau</title>
</head>
<body>
<h1>Morgenblau</h1>
<p>A calm content platform powered by RSS and ATProto. Morgenblau identifies itself
when fetching upstream feeds with a contact address (<a href="mailto:bot@morgen.blue">bot@morgen.blue</a>).</p>
<p>If our fetcher is misbehaving against your server, please reach out.</p>
</body>
</html>
`

// AboutHandler serves the static /about page linked from the fetcher's User-Agent header (SPEC <feed-sources>). Public, no auth.
func AboutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(aboutBody))
	})
}
