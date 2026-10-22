package webfeed

import (
	"context"
	"testing"
)

func TestFetchArticlesMixesCachedAfterUncachedSafely(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(len(postTitles))
	sess := newSession(s.opts())
	var urls []string
	for _, title := range postTitles {
		urls = append(urls, s.url("/blog/"+slugify(title)))
	}
	sess.fetchArticles(context.Background(), urls[len(urls)/2:])
	infos := sess.fetchArticles(context.Background(), urls)
	for _, u := range urls {
		if info := infos[u]; info == nil || !info.ok {
			t.Errorf("%s: info = %+v, want the article", u, info)
		}
	}
}
