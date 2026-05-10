<?php

use App\Models\Feed;
use App\Services\Feeds\FaviconDiscoverer;
use App\Services\Http\DnsResolver;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\Http;
use Tests\Doubles\FakeDnsResolver;

beforeEach(function () {
    Date::setTestNow('2026-05-10 12:00:00');
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver([
        'example.com' => ['93.184.216.34'],
        'blog.example' => ['93.184.216.34'],
    ]));
});

afterEach(fn () => Date::setTestNow());

function fakeHomepage(string $body, int $status = 200): void
{
    Http::fake([
        '*' => function ($request) use ($body, $status) {
            $url = $request->url();

            return match (true) {
                str_contains($url, '/favicon.ico') => Http::response('binary', 200),
                default => Http::response($body, $status, ['Content-Type' => 'text/html']),
            };
        },
    ]);
}

test('persists the apple-touch-icon URL when the homepage advertises one', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example/feed.xml']);

    fakeHomepage('<html><head><link rel="apple-touch-icon" href="/touch.png"></head></html>');

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBe('https://blog.example/touch.png');
    expect($feed->favicon_checked_at)->not->toBeNull();
});

test('prefers SVG icons of equal size over raster', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example/feed.xml']);

    fakeHomepage('<html><head>
        <link rel="icon" sizes="any" type="image/svg+xml" href="/icon.svg">
        <link rel="icon" sizes="any" type="image/png" href="/icon.png">
    </head></html>');

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBe('https://blog.example/icon.svg');
});

test('falls back to /favicon.ico verified via HEAD when HTML has no icon links', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example/feed.xml']);

    fakeHomepage('<html><head><title>nothing here</title></head></html>');

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBe('https://blog.example/favicon.ico');
});

test('persists null when discovery finds no icon at all', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example/feed.xml']);

    Http::fake([
        '*' => fn () => Http::response('<html></html>', 404),
    ]);

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBeNull();
    expect($feed->favicon_checked_at)->not->toBeNull();
});

test('skips re-discovery while favicon_checked_at is fresh', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://blog.example/feed.xml',
        'favicon_url' => 'https://blog.example/sentinel.png',
        'favicon_checked_at' => Date::now()->subDays(5),
    ]);

    Http::fake();
    Http::preventStrayRequests();

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBe('https://blog.example/sentinel.png');
});

test('re-discovers once favicon_checked_at is older than the recheck window', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://blog.example/feed.xml',
        'favicon_url' => 'https://blog.example/old.png',
        'favicon_checked_at' => Date::now()->subDays(31),
    ]);

    fakeHomepage('<html><head><link rel="icon" href="/new.svg" type="image/svg+xml"></head></html>');

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBe('https://blog.example/new.svg');
});

test('persists null without throwing when the feed_url host resolves to a private IP', function () {
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver([
        'private.example' => ['127.0.0.1'],
    ]));

    $feed = Feed::query()->create(['feed_url' => 'https://private.example/feed.xml']);

    Http::fake();

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBeNull();
    expect($feed->favicon_checked_at)->not->toBeNull();
});

test('skips silently when feed_url is malformed', function () {
    $feed = Feed::query()->create(['feed_url' => 'not-a-real-url']);

    Http::fake();
    Http::preventStrayRequests();

    app(FaviconDiscoverer::class)->discover($feed);

    expect($feed->refresh()->favicon_url)->toBeNull();
    expect($feed->favicon_checked_at)->toBeNull();
});
