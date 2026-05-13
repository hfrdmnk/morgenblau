<?php

use App\Data\Feeds\FetchedEntryData;
use App\Exceptions\FeedFetchException;
use App\Services\Feeds\FeedFetcher;
use App\Services\Feeds\FeedIo\Specification as PublishedFirstSpecification;
use App\Services\Feeds\Results\Failed;
use App\Services\Feeds\Results\Gone;
use App\Services\Feeds\Results\Modified;
use App\Services\Feeds\Results\NotModified;
use App\Services\Feeds\Results\RateLimited;
use Carbon\CarbonImmutable;
use FeedIo\FeedIo;
use Psr\Log\NullLogger;
use Tests\Doubles\StubFeedClient;

function feedFetcherWith(StubFeedClient $client): FeedFetcher
{
    $logger = new NullLogger;

    return new FeedFetcher(
        new FeedIo(client: $client, logger: $logger, specification: new PublishedFirstSpecification($logger)),
        $client,
    );
}

function feedFetcherForUrls(array $bodies): FeedFetcher
{
    return feedFetcherWith(new StubFeedClient($bodies));
}

function sampleRssFixture(): string
{
    return (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/sample.rss.xml');
}

test('it parses an RSS feed into a Modified result with FetchedEntryData', function () {
    $fetcher = feedFetcherForUrls(['https://example.com/feed.xml' => sampleRssFixture()]);

    $result = $fetcher->fetch('https://example.com/feed.xml');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries)->toHaveCount(2)
        ->and($result->entries[0])->toBeInstanceOf(FetchedEntryData::class)
        ->and($result->feedTitle)->toBe('Sample Feed');

    $first = $result->entries[0];
    expect($first->title)->toBe('First post')
        ->and($first->link)->toBe('https://example.com/posts/first')
        ->and($first->guid)->toBe('post-1')
        ->and($first->content)->toBe('Short summary of the first post.')
        ->and($first->summary)->toBeNull()
        ->and($first->author)->toBe('Jane Doe')
        ->and($first->publishedAt)->toBeInstanceOf(CarbonImmutable::class)
        ->and($first->publishedAt->toIso8601String())->toBe('2026-04-15T09:30:00+00:00');
});

test('it leaves feedTitle null when the channel has no <title>', function () {
    // Hand-rolled body without a <channel><title> — covers RSS variants that
    // omit it (rare in practice, but our backfill path must handle null).
    $body = <<<'XML'
        <?xml version="1.0" encoding="UTF-8"?>
        <rss version="2.0">
          <channel>
            <link>https://example.com</link>
            <description>Untitled feed</description>
            <item>
              <title>Only item</title>
              <link>https://example.com/posts/one</link>
            </item>
          </channel>
        </rss>
        XML;
    $fetcher = feedFetcherForUrls(['https://example.com/no-title.rss' => $body]);

    $result = $fetcher->fetch('https://example.com/no-title.rss');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->feedTitle)->toBeNull();
});

test('it tolerates items missing optional fields', function () {
    $fetcher = feedFetcherForUrls(['https://example.com/feed.xml' => sampleRssFixture()]);

    $second = $fetcher->fetch('https://example.com/feed.xml')->entries[1];

    expect($second->title)->toBe('Bare item')
        ->and($second->link)->toBe('https://example.com/posts/bare')
        // feed-io falls back to the link when an item has no <guid>
        ->and($second->guid)->toBe('https://example.com/posts/bare')
        ->and($second->summary)->toBeNull()
        ->and($second->content)->toBeNull()
        ->and($second->author)->toBeNull()
        ->and($second->publishedAt)->toBeNull();
});

test('it captures ETag and Last-Modified headers from a 200 response', function () {
    $fetcher = feedFetcherForUrls([
        'https://example.com/feed.xml' => [
            'result' => 'modified',
            'body' => sampleRssFixture(),
            'etag' => 'W/"abc123"',
            'last_modified' => 'Wed, 15 Apr 2026 09:30:00 +0000',
        ],
    ]);

    $result = $fetcher->fetch('https://example.com/feed.xml');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->etag)->toBe('W/"abc123"');
    expect($result->lastModified)->toBe('Wed, 15 Apr 2026 09:30:00 +0000');
});

test('it returns NotModified for a 304 response and forwards stored conditional headers', function () {
    $client = new StubFeedClient([
        'https://example.com/feed.xml' => [
            'result' => 'not_modified',
            'etag' => 'W/"abc123"',
            'last_modified' => 'Wed, 15 Apr 2026 09:30:00 +0000',
        ],
    ]);
    $fetcher = feedFetcherWith($client);

    $result = $fetcher->fetch(
        'https://example.com/feed.xml',
        etag: 'W/"abc123"',
        lastModified: 'Wed, 15 Apr 2026 09:30:00 +0000',
    );

    expect($result)->toBeInstanceOf(NotModified::class);
    expect($result->etag)->toBe('W/"abc123"');
    expect($result->lastModified)->toBe('Wed, 15 Apr 2026 09:30:00 +0000');

    expect($client->lastConditionalHeaders['https://example.com/feed.xml'])->toBe([
        'etag' => 'W/"abc123"',
        'last_modified' => 'Wed, 15 Apr 2026 09:30:00 +0000',
    ]);
});

test('it returns Failed when no fixture is mapped (NotFoundException from client)', function () {
    $fetcher = feedFetcherForUrls([]);

    $result = $fetcher->fetch('https://example.com/missing.xml');

    expect($result)->toBeInstanceOf(Failed::class);
    expect($result->cause)->toBeInstanceOf(FeedFetchException::class);
});

test('it returns Failed for an upstream 404', function () {
    $fetcher = feedFetcherForUrls([
        'https://example.com/feed.xml' => ['result' => 'failed', 'status' => 404],
    ]);

    $result = $fetcher->fetch('https://example.com/feed.xml');

    expect($result)->toBeInstanceOf(Failed::class);
    expect($result->cause->getMessage())->toContain('HTTP 404');
});

test('it returns Failed for a 500 server error', function () {
    $fetcher = feedFetcherForUrls([
        'https://example.com/feed.xml' => ['result' => 'failed', 'status' => 500],
    ]);

    $result = $fetcher->fetch('https://example.com/feed.xml');

    expect($result)->toBeInstanceOf(Failed::class);
    expect($result->cause->getMessage())->toContain('HTTP 500');
});

test('it returns Gone for an HTTP 410', function () {
    $fetcher = feedFetcherForUrls([
        'https://example.com/feed.xml' => ['result' => 'gone'],
    ]);

    $result = $fetcher->fetch('https://example.com/feed.xml');

    expect($result)->toBeInstanceOf(Gone::class);
});

test('it returns RateLimited with parsed numeric Retry-After', function () {
    $fetcher = feedFetcherForUrls([
        'https://example.com/feed.xml' => ['result' => 'rate_limited', 'retry_after' => 90],
    ]);

    $result = $fetcher->fetch('https://example.com/feed.xml');

    expect($result)->toBeInstanceOf(RateLimited::class);
    expect($result->retryAfterSeconds)->toBe(90);
});

test('it returns RateLimited with 0 when Retry-After header is missing', function () {
    $fetcher = feedFetcherForUrls([
        'https://example.com/feed.xml' => ['result' => 'rate_limited'],
    ]);

    $result = $fetcher->fetch('https://example.com/feed.xml');

    expect($result)->toBeInstanceOf(RateLimited::class);
    expect($result->retryAfterSeconds)->toBe(0);
});

test('it parses an Atom 1.0 feed', function () {
    $body = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/atom.xml');
    $fetcher = feedFetcherForUrls(['https://example.com/atom.xml' => $body]);

    $result = $fetcher->fetch('https://example.com/atom.xml');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries)->toHaveCount(2);
    expect($result->entries[0]->title)->toBe('Atom Entry One');
    expect($result->entries[0]->link)->toBe('https://example.com/posts/atom-one');
    expect($result->entries[0]->content)->toContain('Body of the first atom entry');
});

test('it prefers Atom <published> over <updated> for publishedAt', function () {
    // YouTube ships both: <published> is the real upload date; <updated>
    // bumps on view-count refreshes. feed-io's default standard reads
    // whichever element appears last (and YouTube puts <updated> last),
    // which gave us the same "6 days ago" for every recent video.
    $body = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/youtube.atom.xml');
    $fetcher = feedFetcherForUrls(['https://example.com/yt.atom' => $body]);

    $result = $fetcher->fetch('https://example.com/yt.atom');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries[0]->publishedAt?->toIso8601String())->toBe('2026-04-15T09:30:00+00:00');
    expect($result->entries[1]->publishedAt?->toIso8601String())->toBe('2026-04-16T10:00:00+00:00');
});

test('it falls back to <updated> when <published> is absent', function () {
    // Vanilla Atom feeds may carry only <updated>. The PublishedFirst override
    // should preserve the default behavior — fall through to <updated> when
    // there's no <published> sibling.
    $body = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/atom-updated-only.xml');
    $fetcher = feedFetcherForUrls(['https://example.com/atom.xml' => $body]);

    $result = $fetcher->fetch('https://example.com/atom.xml');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries[0]->publishedAt?->toIso8601String())->toBe('2026-04-20T12:00:00+00:00');
});

test('it parses an RSS feed whose <description> carries inline HTML', function () {
    $body = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/content-encoded.rss.xml');
    $fetcher = feedFetcherForUrls(['https://example.com/encoded.rss' => $body]);

    $result = $fetcher->fetch('https://example.com/encoded.rss');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries)->toHaveCount(1);
    expect($result->entries[0]->content)->toContain('<script>')
        ->and($result->entries[0]->content)->toContain('<iframe');
});

test('it prefers <content:encoded> over <description> for the body', function () {
    $body = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/content-encoded-real.rss.xml');
    $fetcher = feedFetcherForUrls(['https://example.com/full.rss' => $body]);

    $result = $fetcher->fetch('https://example.com/full.rss');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries)->toHaveCount(2);

    $full = $result->entries[0];
    expect($full->content)->toContain('First paragraph of the real article.')
        ->and($full->content)->toContain('Second paragraph')
        ->and($full->content)->not->toContain('Short teaser line.');

    // Empty <content:encoded> must not clobber the description fallback.
    $fallback = $result->entries[1];
    expect($fallback->content)->toBe('Description-only body.');
});

test('it falls back to link as guid when no <guid> element is present', function () {
    $body = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/no-guid.rss.xml');
    $fetcher = feedFetcherForUrls(['https://example.com/no-guid.rss' => $body]);

    $result = $fetcher->fetch('https://example.com/no-guid.rss');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries[0]->guid)->toBe('https://example.com/posts/linkable');
    expect($result->entries[1]->guid)->toBe('https://example.com/posts/another');
});

test('it captures audio enclosures from a podcast RSS feed', function () {
    $body = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/podcast.rss.xml');
    $fetcher = feedFetcherForUrls(['https://example.com/podcast.rss' => $body]);

    $result = $fetcher->fetch('https://example.com/podcast.rss');

    expect($result)->toBeInstanceOf(Modified::class);
    expect($result->entries)->toHaveCount(2);

    $first = $result->entries[0];
    expect($first->enclosures)->toHaveCount(1);
    expect($first->enclosures[0]->url)->toBe('https://example.com/audio/ep-1.mp3');
    expect($first->enclosures[0]->type)->toBe('audio/mpeg');
    expect($first->enclosures[0]->length)->toBe(1234567);
});
