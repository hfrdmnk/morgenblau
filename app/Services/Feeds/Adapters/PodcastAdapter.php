<?php

namespace App\Services\Feeds\Adapters;

use App\Data\Feeds\ResolvedFeedData;
use App\Enums\SourceType;
use App\Exceptions\UnresolvableFeedException;
use App\Services\Feeds\FeedAdapter;
use App\Services\Http\OutboundHttpClient;

class PodcastAdapter implements FeedAdapter
{
    private const APPLE_HOST = 'podcasts.apple.com';

    private const SPOTIFY_HOST = 'open.spotify.com';

    public function __construct(private readonly OutboundHttpClient $http) {}

    public function claims(string $url): bool
    {
        $host = $this->host($url);

        return $this->matches($host, self::APPLE_HOST)
            || $this->matches($host, self::SPOTIFY_HOST);
    }

    /**
     * @return non-empty-list<ResolvedFeedData>
     */
    public function resolve(string $url): array
    {
        if ($this->matches($this->host($url), self::SPOTIFY_HOST)) {
            throw new UnresolvableFeedException(
                "Spotify-only podcasts can't be subscribed to yet. Try the show's website RSS or its Apple Podcasts page."
            );
        }

        return [$this->resolveApple($url)];
    }

    private function host(string $url): ?string
    {
        $host = parse_url($url, PHP_URL_HOST);

        return is_string($host) ? strtolower($host) : null;
    }

    private function matches(?string $host, string $domain): bool
    {
        return $host !== null && ($host === $domain || str_ends_with($host, '.'.$domain));
    }

    private function resolveApple(string $url): ResolvedFeedData
    {
        if (preg_match('#/id(\d+)#', $url, $matches) !== 1) {
            throw new UnresolvableFeedException("Couldn't extract an Apple podcast ID from {$url}.");
        }

        $podcastId = $matches[1];

        $response = $this->http->getTrusted('https://itunes.apple.com/lookup', [
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

        return new ResolvedFeedData(
            feedUrl: $result['feedUrl'],
            title: $result['collectionName'] ?? $result['trackName'] ?? null,
            siteUrl: $result['collectionViewUrl'] ?? $url,
            sourceType: SourceType::Podcast,
        );
    }
}
