<?php

namespace App\Services\FeedAdapters;

use App\Services\FeedAdapters\Exceptions\UnresolvableFeedException;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Str;

class PodcastAdapter implements FeedAdapter
{
    public function tryResolve(string $url): ?ResolvedFeed
    {
        $host = parse_url($url, PHP_URL_HOST);
        if ($host === null) {
            return null;
        }

        if (Str::contains($host, 'podcasts.apple.com')) {
            return $this->resolveApple($url);
        }

        if (Str::contains($host, 'open.spotify.com')) {
            throw new UnresolvableFeedException(
                "Spotify-only podcasts can't be subscribed to yet. Try the show's website RSS or its Apple Podcasts page."
            );
        }

        return null;
    }

    private function resolveApple(string $url): ResolvedFeed
    {
        if (preg_match('#/id(\d+)#', $url, $matches) !== 1) {
            throw new UnresolvableFeedException("Couldn't extract an Apple podcast ID from {$url}.");
        }

        $podcastId = $matches[1];

        $response = Http::timeout(10)
            ->get('https://itunes.apple.com/lookup', [
                'id' => $podcastId,
                'entity' => 'podcast',
            ]);

        if ($response->failed()) {
            throw new UnresolvableFeedException("iTunes lookup failed for podcast {$podcastId}.");
        }

        $result = $response->json('results.0');

        if (! is_array($result) || empty($result['feedUrl'])) {
            throw new UnresolvableFeedException("No public RSS feed found for Apple podcast {$podcastId}.");
        }

        return new ResolvedFeed(
            feedUrl: $result['feedUrl'],
            title: $result['collectionName'] ?? $result['trackName'] ?? null,
            siteUrl: $result['collectionViewUrl'] ?? $url,
            category: 'source:podcast',
        );
    }
}
