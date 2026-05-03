<?php

namespace App\Services\Feeds\Adapters;

use App\Data\Feeds\ResolvedFeedData;
use App\Enums\SourceType;
use App\Services\Feeds\Exceptions\UnresolvableFeedException;
use App\Services\Feeds\FeedAdapter;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Str;

class YouTubeAdapter implements FeedAdapter
{
    private const FEED_URL = 'https://www.youtube.com/feeds/videos.xml?channel_id=';

    /**
     * @return list<ResolvedFeedData>
     */
    public function tryResolve(string $url): array
    {
        $host = parse_url($url, PHP_URL_HOST);
        if ($host === null || ! Str::endsWith($host, ['youtube.com', 'youtu.be'])) {
            return [];
        }

        // SOCS=CAI bypasses YouTube's EU consent redirect, which otherwise serves a metadata-free page.
        $response = Http::timeout(10)
            ->withHeaders(['Cookie' => 'SOCS=CAI'])
            ->get($url);

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

        return [new ResolvedFeedData(
            feedUrl: self::FEED_URL.$channelId,
            title: $title,
            siteUrl: $url,
            sourceType: SourceType::Video,
        )];
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
        // Canonical link and itemprop name the page's own channel; "channelId" literals also appear in the sidebar.
        if (preg_match('#<link\s+rel="canonical"\s+href="https?://(?:www\.)?youtube\.com/channel/(UC[A-Za-z0-9_-]{20,})"#', $body, $matches) === 1) {
            return $matches[1];
        }

        if (preg_match('#<meta itemprop="(?:channelId|identifier)"\s+content="(UC[A-Za-z0-9_-]{20,})"#', $body, $matches) === 1) {
            return $matches[1];
        }

        if (preg_match('#"externalId":"(UC[A-Za-z0-9_-]{20,})"#', $body, $matches) === 1) {
            return $matches[1];
        }

        if (preg_match('#"channelId":"(UC[A-Za-z0-9_-]{20,})"#', $body, $matches) === 1) {
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
