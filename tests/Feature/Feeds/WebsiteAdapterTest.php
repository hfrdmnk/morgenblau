<?php

use App\Exceptions\UnresolvableFeedException;
use App\Services\Feeds\Adapters\WebsiteAdapter;
use App\Services\Http\DnsResolver;
use Illuminate\Support\Facades\Http;
use Tests\Doubles\FakeDnsResolver;

beforeEach(function () {
    Http::preventStrayRequests();
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver([
        'example.com' => ['93.184.216.34'],
    ]));
});

test('claims any http(s) URL', function () {
    $adapter = app(WebsiteAdapter::class);

    expect($adapter->claims('https://example.com'))->toBeTrue();
    expect($adapter->claims('http://example.com'))->toBeTrue();
    expect($adapter->claims('javascript:alert(1)'))->toBeFalse();
    expect($adapter->claims('ftp://example.com'))->toBeFalse();
});

test('resolves a direct XML feed URL into one candidate', function () {
    Http::fake([
        'example.com/feed.xml' => Http::response(
            '<?xml version="1.0"?><rss><channel><title>Direct Feed</title></channel></rss>',
            200,
            ['Content-Type' => 'application/rss+xml'],
        ),
    ]);

    $candidates = app(WebsiteAdapter::class)->resolve('https://example.com/feed.xml');

    expect($candidates)->toHaveCount(1)
        ->and($candidates[0]->feedUrl)->toBe('https://example.com/feed.xml')
        ->and($candidates[0]->title)->toBe('Direct Feed')
        ->and($candidates[0]->siteUrl)->toBeNull();
});

test('detects an XML feed even when content-type lies as text/html', function () {
    Http::fake([
        'example.com/feed' => Http::response(
            '<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>By Body</title></feed>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    expect(app(WebsiteAdapter::class)->resolve('https://example.com/feed'))->toHaveCount(1);
});

test('throws when the page advertises no feed', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><title>Plain</title></head><body>Nothing.</body></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    expect(fn () => app(WebsiteAdapter::class)->resolve('https://example.com'))
        ->toThrow(UnresolvableFeedException::class, 'No RSS or Atom feed advertised');
});

test('throws when the upstream fetch fails', function () {
    Http::fake(['example.com' => Http::response('', 502)]);

    expect(fn () => app(WebsiteAdapter::class)->resolve('https://example.com'))
        ->toThrow(UnresolvableFeedException::class);
});
