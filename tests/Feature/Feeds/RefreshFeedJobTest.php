<?php

use App\Data\Feeds\FetchedEntryData;
use App\Exceptions\FeedFetchException;
use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\FeedEntry;
use App\Services\Feeds\FeedEntryUpserter;
use App\Services\Feeds\FeedFetcher;
use App\Services\Feeds\Results\Modified;
use App\Services\Feeds\Results\NotModified;
use Carbon\CarbonImmutable;
use Illuminate\Contracts\Queue\ShouldBeUnique;
use Illuminate\Support\Facades\Date;

test('on success updates last_fetched_at, clears last_dispatched_at, and writes entries', function () {
    Date::setTestNow('2026-05-07 12:00:00');

    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'last_dispatched_at' => Date::now()->subMinute(),
    ]);

    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')
        ->once()
        ->with('https://example.com/rss', null, null)
        ->andReturn(new Modified(
            entries: [
                new FetchedEntryData(
                    title: 'Hello',
                    link: 'https://example.com/hello',
                    guid: 'urn:1',
                    summary: null,
                    content: null,
                    author: null,
                    publishedAt: CarbonImmutable::parse('2026-05-07 11:00:00'),
                ),
            ],
            etag: null,
            lastModified: null,
        ));
    app()->instance(FeedFetcher::class, $fetcher);

    (new RefreshFeedJob($feed->id))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));

    $feed->refresh();
    expect($feed->last_fetched_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->last_dispatched_at)->toBeNull();
    expect($feed->feedEntries()->count())->toBe(1);
});

test('on success clears any previously recorded failure state', function () {
    Date::setTestNow('2026-05-07 12:00:00');

    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'last_dispatched_at' => Date::now()->subMinute(),
        'last_failed_at' => Date::now()->subHour(),
        'last_error' => 'something broke earlier',
    ]);

    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')
        ->once()
        ->andReturn(new Modified(entries: [], etag: null, lastModified: null));
    app()->instance(FeedFetcher::class, $fetcher);

    (new RefreshFeedJob($feed->id))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));

    $feed->refresh();
    expect($feed->last_failed_at)->toBeNull();
    expect($feed->last_error)->toBeNull();
});

test('on Modified result it persists rotated etag and last_modified headers', function () {
    Date::setTestNow('2026-05-07 12:00:00');

    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'etag_header' => 'W/"old"',
        'last_modified_header' => 'Tue, 14 Apr 2026 09:00:00 +0000',
    ]);

    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')
        ->once()
        ->andReturn(new Modified(
            entries: [],
            etag: 'W/"new"',
            lastModified: 'Wed, 15 Apr 2026 09:30:00 +0000',
        ));
    app()->instance(FeedFetcher::class, $fetcher);

    (new RefreshFeedJob($feed->id))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));

    $feed->refresh();
    expect($feed->etag_header)->toBe('W/"new"');
    expect($feed->last_modified_header)->toBe('Wed, 15 Apr 2026 09:30:00 +0000');
});

test('it forwards stored cache headers into the fetcher call', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'etag_header' => 'W/"abc"',
        'last_modified_header' => 'Wed, 15 Apr 2026 09:30:00 +0000',
    ]);

    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')
        ->once()
        ->with('https://example.com/rss', 'W/"abc"', 'Wed, 15 Apr 2026 09:30:00 +0000')
        ->andReturn(new Modified(entries: [], etag: 'W/"abc"', lastModified: 'Wed, 15 Apr 2026 09:30:00 +0000'));
    app()->instance(FeedFetcher::class, $fetcher);

    (new RefreshFeedJob($feed->id))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));
});

test('on NotModified it advances last_fetched_at, clears dispatched/failure, and persists no entries', function () {
    Date::setTestNow('2026-05-07 12:00:00');

    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'last_dispatched_at' => Date::now()->subMinute(),
        'last_failed_at' => Date::now()->subHour(),
        'last_error' => 'previous error',
        'etag_header' => 'W/"abc"',
        'last_modified_header' => 'Wed, 15 Apr 2026 09:30:00 +0000',
    ]);

    FeedEntry::query()->create([
        'feed_id' => $feed->id,
        'guid' => 'urn:existing',
        'title' => 'Existing',
        'link' => 'https://example.com/existing',
        'first_seen_at' => Date::now()->subDay(),
        'updated_at' => Date::now()->subDay(),
    ]);

    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')
        ->once()
        ->andReturn(new NotModified(etag: 'W/"abc"', lastModified: 'Wed, 15 Apr 2026 09:30:00 +0000'));
    app()->instance(FeedFetcher::class, $fetcher);

    (new RefreshFeedJob($feed->id))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));

    $feed->refresh();
    expect($feed->last_fetched_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->last_dispatched_at)->toBeNull();
    expect($feed->last_failed_at)->toBeNull();
    expect($feed->last_error)->toBeNull();
    expect($feed->feedEntries()->count())->toBe(1);
});

test('returns silently when the feed has been deleted', function () {
    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldNotReceive('fetch');
    app()->instance(FeedFetcher::class, $fetcher);

    (new RefreshFeedJob(999_999))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));

    expect(true)->toBeTrue();
});

test('on failure records last_failed_at, last_error, clears last_dispatched_at, leaves last_fetched_at untouched, and re-throws', function () {
    Date::setTestNow('2026-05-07 12:00:00');

    $previousFetchedAt = Date::now()->subDay();

    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'last_dispatched_at' => Date::now()->subMinute(),
        'last_fetched_at' => $previousFetchedAt,
    ]);

    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')
        ->once()
        ->andThrow(new FeedFetchException('boom'));
    app()->instance(FeedFetcher::class, $fetcher);

    expect(fn () => (new RefreshFeedJob($feed->id))->handle(
        app(FeedFetcher::class),
        app(FeedEntryUpserter::class),
    ))->toThrow(FeedFetchException::class, 'boom');

    $feed->refresh();
    expect($feed->last_failed_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->last_error)->toBe('boom');
    expect($feed->last_dispatched_at)->toBeNull();
    expect($feed->last_fetched_at?->toDateTimeString())->toBe($previousFetchedAt->toDateTimeString());
});

test('truncates last_error to 500 characters', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
    ]);

    $longMessage = str_repeat('a', 600);

    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')
        ->once()
        ->andThrow(new FeedFetchException($longMessage));
    app()->instance(FeedFetcher::class, $fetcher);

    try {
        (new RefreshFeedJob($feed->id))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));
    } catch (FeedFetchException) {
        // expected
    }

    $feed->refresh();
    expect(strlen((string) $feed->last_error))->toBe(500);
});

test('uses a single try (no automatic retry on failure)', function () {
    expect((new RefreshFeedJob(1))->tries)->toBe(1);
});

test('declares uniqueness keyed by feed id with a 5 minute window', function () {
    $job = new RefreshFeedJob(42);

    expect($job)->toBeInstanceOf(ShouldBeUnique::class);
    expect($job->uniqueId())->toBe('42');
    expect($job->uniqueFor)->toBe(300);
});
