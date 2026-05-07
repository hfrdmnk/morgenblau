<?php

use App\Data\Feeds\FetchedEntryData;
use App\Exceptions\FeedFetchException;
use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Services\Feeds\FeedEntryUpserter;
use App\Services\Feeds\FeedFetcher;
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
        ->with('https://example.com/rss')
        ->andReturn([
            new FetchedEntryData(
                title: 'Hello',
                link: 'https://example.com/hello',
                guid: 'urn:1',
                summary: null,
                content: null,
                author: null,
                publishedAt: CarbonImmutable::parse('2026-05-07 11:00:00'),
            ),
        ]);
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
        ->andReturn([]);
    app()->instance(FeedFetcher::class, $fetcher);

    (new RefreshFeedJob($feed->id))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));

    $feed->refresh();
    expect($feed->last_failed_at)->toBeNull();
    expect($feed->last_error)->toBeNull();
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
