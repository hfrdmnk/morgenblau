<?php

namespace App\Services\FeedAdapters;

use App\Services\FeedAdapters\Exceptions\UnresolvableFeedException;
use DOMDocument;
use DOMXPath;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Str;

class WebsiteAdapter implements FeedAdapter
{
    private const FEED_CONTENT_TYPES = [
        'application/rss+xml',
        'application/atom+xml',
        'application/xml',
        'text/xml',
    ];

    private const FEED_LINK_TYPES = [
        'application/rss+xml',
        'application/atom+xml',
    ];

    public function tryResolve(string $url): ?ResolvedFeed
    {
        $scheme = parse_url($url, PHP_URL_SCHEME);
        if (! in_array($scheme, ['http', 'https'], true)) {
            return null;
        }

        $response = Http::timeout(10)->get($url);

        if ($response->failed()) {
            throw new UnresolvableFeedException("Couldn't fetch {$url} (HTTP {$response->status()}).");
        }

        $contentType = strtolower((string) $response->header('Content-Type'));
        $body = $response->body();

        if ($this->looksLikeFeed($contentType, $body)) {
            return $this->resolveDirectFeed($url, $body);
        }

        return $this->resolveFromHtml($url, $body);
    }

    private function looksLikeFeed(string $contentType, string $body): bool
    {
        foreach (self::FEED_CONTENT_TYPES as $type) {
            if (Str::startsWith($contentType, $type)) {
                return true;
            }
        }

        $head = ltrim(substr($body, 0, 200));

        return Str::startsWith($head, ['<?xml', '<rss', '<feed', '<rdf:RDF']);
    }

    private function resolveDirectFeed(string $url, string $body): ResolvedFeed
    {
        return new ResolvedFeed(
            feedUrl: $url,
            title: $this->extractFeedTitle($body),
            siteUrl: null,
            category: 'source:blog',
        );
    }

    private function resolveFromHtml(string $url, string $body): ResolvedFeed
    {
        $xpath = $this->buildXPath($body);
        $feedUrl = $this->findFeedLink($xpath, $url);

        if ($feedUrl === null) {
            throw new UnresolvableFeedException("No RSS or Atom feed advertised on {$url}.");
        }

        return new ResolvedFeed(
            feedUrl: $feedUrl,
            title: $this->extractHtmlTitle($xpath),
            siteUrl: $url,
            category: 'source:blog',
        );
    }

    private function buildXPath(string $body): DOMXPath
    {
        $document = new DOMDocument;
        $previousErrors = libxml_use_internal_errors(true);

        try {
            $document->loadHTML($body, LIBXML_NOERROR | LIBXML_NOWARNING);
        } finally {
            libxml_clear_errors();
            libxml_use_internal_errors($previousErrors);
        }

        return new DOMXPath($document);
    }

    private function findFeedLink(DOMXPath $xpath, string $base): ?string
    {
        $nodes = $xpath->query('//link[@rel="alternate"]');
        if ($nodes === false) {
            return null;
        }

        foreach ($nodes as $node) {
            $type = strtolower((string) $node->getAttribute('type'));
            $href = trim((string) $node->getAttribute('href'));

            if ($href === '' || ! in_array($type, self::FEED_LINK_TYPES, true)) {
                continue;
            }

            return $this->absolutize($href, $base);
        }

        return null;
    }

    private function extractFeedTitle(string $body): ?string
    {
        if (preg_match('#<title[^>]*>([^<]+)</title>#i', $body, $matches) === 1) {
            return html_entity_decode(trim($matches[1]), ENT_QUOTES | ENT_HTML5);
        }

        return null;
    }

    private function extractHtmlTitle(DOMXPath $xpath): ?string
    {
        $og = $xpath->query('//meta[@property="og:site_name"]/@content');
        if ($og !== false && $og->length > 0) {
            $value = trim((string) $og->item(0)->nodeValue);
            if ($value !== '') {
                return $value;
            }
        }

        $title = $xpath->query('//title');
        if ($title !== false && $title->length > 0) {
            $value = trim((string) $title->item(0)->textContent);
            if ($value !== '') {
                return $value;
            }
        }

        return null;
    }

    private function absolutize(string $href, string $base): string
    {
        if (preg_match('#^https?://#i', $href) === 1) {
            return $href;
        }

        $parts = parse_url($base);
        if ($parts === false || ! isset($parts['scheme'], $parts['host'])) {
            return $href;
        }

        $origin = $parts['scheme'].'://'.$parts['host'];
        if (isset($parts['port'])) {
            $origin .= ':'.$parts['port'];
        }

        if (Str::startsWith($href, '/')) {
            return $origin.$href;
        }

        $path = $parts['path'] ?? '/';
        $dir = rtrim(substr($path, 0, (int) strrpos($path, '/') + 1), '/');

        return $origin.$dir.'/'.$href;
    }
}
