<?php

use App\Exceptions\FeedFetchException;
use App\Models\Feed;
use Illuminate\Support\Facades\Date;

beforeEach(function () {
    Date::setTestNow('2026-05-07 12:00:00');
});

afterEach(function () {
    Date::setTestNow();
});

test('markDispatched stamps last_dispatched_at to now', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'last_dispatched_at' => null,
    ]);

    $feed->markDispatched();

    expect($feed->fresh()->last_dispatched_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
});

test('markFetched clears dispatched/failure state and writes cache headers', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'last_dispatched_at' => Date::now()->subMinute(),
        'last_failed_at' => Date::now()->subHour(),
        'last_error' => 'old error',
        'consecutive_failures' => 3,
        'disabled_at' => Date::now()->subDay(),
        'etag_header' => 'W/"old"',
        'last_modified_header' => 'old date',
    ]);

    $feed->markFetched('W/"new"', 'Wed, 07 May 2026 11:00:00 +0000');

    $feed->refresh();
    expect($feed->last_fetched_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->last_dispatched_at)->toBeNull();
    expect($feed->last_failed_at)->toBeNull();
    expect($feed->last_error)->toBeNull();
    expect($feed->etag_header)->toBe('W/"new"');
    expect($feed->last_modified_header)->toBe('Wed, 07 May 2026 11:00:00 +0000');
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 12:30:00');
    expect($feed->consecutive_failures)->toBe(0);
    expect($feed->disabled_at)->toBeNull();
});

test('markNotModified preserves cache headers, advances next_check_at, and clears failure state', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'etag_header' => 'W/"keep"',
        'last_modified_header' => 'kept date',
        'last_dispatched_at' => Date::now()->subMinute(),
        'last_failed_at' => Date::now()->subHour(),
        'last_error' => 'previous error',
        'consecutive_failures' => 4,
        'disabled_at' => Date::now()->subWeek(),
    ]);

    $feed->markNotModified();

    $feed->refresh();
    expect($feed->last_fetched_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->last_dispatched_at)->toBeNull();
    expect($feed->last_failed_at)->toBeNull();
    expect($feed->last_error)->toBeNull();
    expect($feed->etag_header)->toBe('W/"keep"');
    expect($feed->last_modified_header)->toBe('kept date');
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 12:30:00');
    expect($feed->consecutive_failures)->toBe(0);
    expect($feed->disabled_at)->toBeNull();
});

test('markFailed increments counter, clears dispatched, and uses backoff ladder', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 2,
        'last_dispatched_at' => Date::now()->subMinute(),
    ]);

    $feed->markFailed(new FeedFetchException('boom'));

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe(3);
    expect($feed->last_failed_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->last_error)->toBe('boom');
    expect($feed->last_dispatched_at)->toBeNull();
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 13:00:00');
    expect($feed->disabled_at)->toBeNull();
});

test('markFailed mutes the feed at the 20-failure threshold', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 19,
    ]);

    $feed->markFailed(new FeedFetchException('boom'));

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe(20);
    expect($feed->disabled_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
});

test('markRateLimited honors Retry-After when larger than the backoff floor', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 0,
    ]);

    $feed->markRateLimited(600);

    $feed->refresh();
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 12:10:00');
    expect($feed->consecutive_failures)->toBe(0);
    expect($feed->last_error)->toBe('HTTP 429 (retry after 600s)');
});

test('markRateLimited falls back to backoff floor when Retry-After is shorter', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 3,
    ]);

    $feed->markRateLimited(10);

    $feed->refresh();
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 13:00:00');
    expect($feed->consecutive_failures)->toBe(3);
});

test('markGone disables the feed without touching the failure counter', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 2,
    ]);

    $feed->markGone();

    $feed->refresh();
    expect($feed->disabled_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->consecutive_failures)->toBe(2);
    expect($feed->last_error)->toBe('HTTP 410 Gone');
});
