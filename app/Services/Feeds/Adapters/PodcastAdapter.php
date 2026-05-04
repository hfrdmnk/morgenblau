<?php

namespace App\Services\Feeds\Adapters;

use App\Data\Feeds\ResolvedFeedData;
use App\Enums\SourceType;
use App\Exceptions\UnresolvableFeedException;
use App\Services\Feeds\FeedAdapter;
use App\Services\Http\OutboundHttpClient;
use Illuminate\Support\Str;

class PodcastAdapter implements FeedAdapter
{
    public function __construct(private readonly OutboundHttpClient $http) {}

    public function claims(string $url): bool
    {
        $host = parse_url($url, PHP_URL_HOST);
        if (! is_string($host)) {
            return false;
        }

        return Str::contains($host, ['podcasts.apple.com', 'open.spotify.com']);
    }

    /**
     * @return non-empty-list<ResolvedFeedData>
     */
    public function resolve(string $url): array
    {
        $host = (string) parse_url($url, PHP_URL_HOST);

        if (Str::contains($host, 'open.spotify.com')) {
            throw new UnresolvableFeedException(
                "Spotify-only podcasts can't be subscribed to yet. Try the show's website RSS or its Apple Podcasts page."
            );
        }

        return [$this->resolveApple($url)];
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
