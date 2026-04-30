<?php

namespace App\Services\FeedAdapters;

use App\Services\FeedAdapters\Exceptions\UnresolvableFeedException;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Str;

class YouTubeAdapter implements FeedAdapter
{
    private const FEED_URL = 'https://www.youtube.com/feeds/videos.xml?channel_id=';

    public function tryResolve(string $url): ?ResolvedFeed
    {
        $host = parse_url($url, PHP_URL_HOST);
        if ($host === null || ! Str::endsWith($host, ['youtube.com', 'youtu.be'])) {
            return null;
        }

        $response = Http::timeout(10)->get($url);

        if ($response->failed()) {
            throw new UnresolvableFeedException("Couldn't fetch {$url} (HTTP {$response->status()}).");
        }

        $body = $response->body();

        $channelId = $this->extractChannelIdFromUrl($url) ?? $this->extractChannelIdFromBody($body);

        if ($channelId === null) {
            throw new UnresolvableFeedException(
                "Couldn't resolve a YouTube channel ID from {$url}. Try pasting the channel's RSS URL directly."
            );
        }

        $title = $this->extractTitle($body) ?? "YouTube channel {$channelId}";

        return new ResolvedFeed(
            feedUrl: self::FEED_URL.$channelId,
            title: $title,
            siteUrl: $url,
            category: 'source:video',
        );
    }

    private function extractChannelIdFromUrl(string $url): ?string
    {
        if (preg_match('#/channel/(UC[A-Za-z0-9_-]{20,})#', $url, $matches) === 1) {
            return $matches[1];
        }

        return null;
    }

    private function extractChannelIdFromBody(string $body): ?string
    {
        if (preg_match('#"channelId":"(UC[A-Za-z0-9_-]{20,})"#', $body, $matches) === 1) {
            return $matches[1];
        }

        if (preg_match('#<meta itemprop="(?:channelId|identifier)"\s+content="(UC[A-Za-z0-9_-]{20,})"#', $body, $matches) === 1) {
            return $matches[1];
        }

        return null;
    }

    private function extractTitle(string $body): ?string
    {
        if (preg_match('#<meta\s+property="og:title"\s+content="([^"]+)"#', $body, $matches) === 1) {
            return html_entity_decode($matches[1], ENT_QUOTES | ENT_HTML5);
        }

        if (preg_match('#<title>([^<]+)</title>#', $body, $matches) === 1) {
            return html_entity_decode(trim($matches[1]), ENT_QUOTES | ENT_HTML5);
        }

        return null;
    }
}
