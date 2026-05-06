<?php

use App\Exceptions\UnsafeUrlException;
use App\Services\Feeds\OutboundFeedClient;
use App\Services\Http\DnsResolver;
use FeedIo\Adapter\NotFoundException;
use FeedIo\Adapter\ServerErrorException;
use Illuminate\Support\Facades\Http;
use Tests\Doubles\FakeDnsResolver;

beforeEach(function () {
    Http::preventStrayRequests();
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver([
        'example.com' => ['93.184.216.34'],
    ]));
});

test('returns a feed-io ResponseInterface for 200 responses', function () {
    Http::fake([
        'example.com/feed.xml' => Http::response('<rss/>', 200, [
            'Last-Modified' => 'Wed, 15 Apr 2026 09:30:00 +0000',
        ]),
    ]);

    $response = app(OutboundFeedClient::class)->getResponse('https://example.com/feed.xml');

    expect($response->getStatusCode())->toBe(200)
        ->and($response->getBody())->toBe('<rss/>')
        ->and($response->getLastModified()?->format(DateTime::RFC2822))
        ->toBe('Wed, 15 Apr 2026 09:30:00 +0000');
});

test('throws NotFoundException on 404', function () {
    Http::fake([
        'example.com/missing.xml' => Http::response('', 404),
    ]);

    expect(fn () => app(OutboundFeedClient::class)->getResponse('https://example.com/missing.xml'))
        ->toThrow(NotFoundException::class);
});

test('throws ServerErrorException on 500', function () {
    Http::fake([
        'example.com/broken.xml' => Http::response('boom', 500),
    ]);

    expect(fn () => app(OutboundFeedClient::class)->getResponse('https://example.com/broken.xml'))
        ->toThrow(ServerErrorException::class);
});

test('forwards If-Modified-Since when modifiedSince is provided', function () {
    Http::fake([
        'example.com/feed.xml' => Http::response('<rss/>', 200),
    ]);

    $since = new DateTime('2026-04-15T09:30:00+0000');
    app(OutboundFeedClient::class)->getResponse('https://example.com/feed.xml', $since);

    Http::assertSent(fn ($request) => $request->hasHeader('If-Modified-Since', $since->format(DateTime::RFC2822)));
});

test('lets UnsafeUrlException bubble so FeedFetcher can surface the SSRF reason', function () {
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver([
        'rebound.example' => ['127.0.0.1'],
    ]));

    expect(fn () => app(OutboundFeedClient::class)->getResponse('http://rebound.example/feed.xml'))
        ->toThrow(UnsafeUrlException::class);
});
