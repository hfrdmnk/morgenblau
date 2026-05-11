<?php

use App\Exceptions\UnresolvableFeedException;
use App\Services\Feeds\Adapters\YouTubeAdapter;
use App\Services\Http\DnsResolver;
use Illuminate\Support\Facades\Http;
use Tests\Doubles\FakeDnsResolver;

beforeEach(function () {
    Http::preventStrayRequests();
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver([
        'www.youtube.com' => ['142.250.0.46'],
        'youtube.com' => ['142.250.0.46'],
    ]));
});

test('claims YouTube hosts only', function () {
    $adapter = app(YouTubeAdapter::class);

    expect($adapter->claims('https://www.youtube.com/channel/UC...'))->toBeTrue();
    expect($adapter->claims('https://youtu.be/abc'))->toBeTrue();
    expect($adapter->claims('https://example.com'))->toBeFalse();
});

test('extracts channel ID from a /channel/ URL via the response body', function () {
    Http::fake([
        '*youtube.com*' => Http::response(
            '<html><meta property="og:title" content="Marques Brownlee"></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $candidates = app(YouTubeAdapter::class)->resolve(
        'https://www.youtube.com/channel/UCBJycsmduvYEL83R_U4JriQ',
    );

    expect($candidates)->toHaveCount(1)
        ->and($candidates[0]->feedUrl)
        ->toBe('https://www.youtube.com/feeds/videos.xml?channel_id=UCBJycsmduvYEL83R_U4JriQ');
});

test('throws when no channel ID can be resolved', function () {
    Http::fake([
        '*youtube.com*' => Http::response(
            '<html><body>Cookie wall.</body></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    expect(fn () => app(YouTubeAdapter::class)->resolve('https://www.youtube.com/@unknownmaybe'))
        ->toThrow(UnresolvableFeedException::class, "Couldn't resolve a YouTube channel ID");
});
