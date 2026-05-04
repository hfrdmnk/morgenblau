<?php

use App\Enums\SourceType;
use App\Exceptions\UnresolvableFeedException;
use App\Services\Feeds\Adapters\PodcastAdapter;
use Illuminate\Support\Facades\Http;

beforeEach(function () {
    Http::preventStrayRequests();
});

test('claims Apple and Spotify hosts only', function () {
    $adapter = app(PodcastAdapter::class);

    expect($adapter->claims('https://podcasts.apple.com/show/x/id1'))->toBeTrue();
    expect($adapter->claims('https://open.spotify.com/show/abc'))->toBeTrue();
    expect($adapter->claims('https://example.com'))->toBeFalse();
});

test('does not claim hosts that merely contain the matched domain as a substring', function () {
    $adapter = app(PodcastAdapter::class);

    expect($adapter->claims('https://podcasts.apple.com.evil.example/'))->toBeFalse();
    expect($adapter->claims('https://notpodcasts.apple.com/'))->toBeFalse();
    expect($adapter->claims('https://open.spotify.com.evil.example/'))->toBeFalse();
});

test('resolves an Apple Podcasts URL via iTunes lookup', function () {
    Http::fake([
        'itunes.apple.com/lookup*' => Http::response([
            'resultCount' => 1,
            'results' => [[
                'feedUrl' => 'https://example.com/podcast.rss',
                'collectionName' => 'Calm Mornings',
                'collectionViewUrl' => 'https://podcasts.apple.com/us/podcast/calm-mornings/id123',
            ]],
        ], 200),
    ]);

    $candidates = app(PodcastAdapter::class)->resolve(
        'https://podcasts.apple.com/us/podcast/calm-mornings/id123',
    );

    expect($candidates)->toHaveCount(1)
        ->and($candidates[0]->feedUrl)->toBe('https://example.com/podcast.rss')
        ->and($candidates[0]->title)->toBe('Calm Mornings')
        ->and($candidates[0]->sourceType)->toBe(SourceType::Podcast);
});

test('throws when iTunes lookup omits feedUrl', function () {
    Http::fake([
        'itunes.apple.com/lookup*' => Http::response([
            'resultCount' => 1,
            'results' => [['collectionName' => 'Spotify-only']],
        ], 200),
    ]);

    expect(fn () => app(PodcastAdapter::class)->resolve(
        'https://podcasts.apple.com/us/podcast/x/id999',
    ))->toThrow(UnresolvableFeedException::class, 'No public RSS feed');
});

test('throws when iTunes lookup HTTP fails', function () {
    Http::fake(['itunes.apple.com/lookup*' => Http::response('', 503)]);

    expect(fn () => app(PodcastAdapter::class)->resolve(
        'https://podcasts.apple.com/us/podcast/x/id123',
    ))->toThrow(UnresolvableFeedException::class, 'iTunes lookup failed');
});

test('rejects Spotify URLs with a helpful message', function () {
    expect(fn () => app(PodcastAdapter::class)->resolve('https://open.spotify.com/show/abc'))
        ->toThrow(UnresolvableFeedException::class, "Spotify-only podcasts can't be subscribed to");
});
