<?php

use App\Exceptions\FeedFetchException;
use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use App\Services\Feeds\FeedEntryUpserter;
use App\Services\Feeds\FeedFetcher;
use App\Services\Feeds\Results\Failed;
use App\Services\Feeds\Results\FetchedFeedResult;
use App\Services\Feeds\Results\Gone;
use App\Services\Feeds\Results\Modified;
use App\Services\Feeds\Results\NotModified;
use App\Services\Feeds\Results\RateLimited;
use Illuminate\Console\Scheduling\Event;
use Illuminate\Console\Scheduling\Schedule;
use Illuminate\Support\Facades\Artisan;
use Illuminate\Support\Facades\Bus;
use Illuminate\Support\Facades\Date;

beforeEach(function () {
    Date::setTestNow('2026-05-07 12:00:00');
});

afterEach(function () {
    Date::setTestNow();
});

function bindFetcherReturning(FetchedFeedResult $result): void
{
    $fetcher = Mockery::mock(FeedFetcher::class);
    $fetcher->shouldReceive('fetch')->andReturn($result);
    app()->instance(FeedFetcher::class, $fetcher);
}

function runJob(int $feedId): void
{
    (new RefreshFeedJob($feedId))->handle(app(FeedFetcher::class), app(FeedEntryUpserter::class));
}

function makeFeedSubscriber(Feed $feed): User
{
    $user = User::factory()->create();
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);

    return $user;
}

test('Failed result advances next_check_at by the backoff ladder step for the new counter value', function (int $startCounter, string $expectedNextCheck) {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => $startCounter,
    ]);

    bindFetcherReturning(new Failed(new FeedFetchException('boom')));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe($startCounter + 1);
    expect($feed->next_check_at?->toDateTimeString())->toBe($expectedNextCheck);
})->with([
    'counter 0 → 1 → 5min' => [0, '2026-05-07 12:05:00'],
    'counter 1 → 2 → 15min' => [1, '2026-05-07 12:15:00'],
    'counter 2 → 3 → 1h' => [2, '2026-05-07 13:00:00'],
    'counter 3 → 4 → 6h' => [3, '2026-05-07 18:00:00'],
    'counter 4 → 5 → 24h cap' => [4, '2026-05-08 12:00:00'],
    'counter 10 → 11 → still 24h cap' => [10, '2026-05-08 12:00:00'],
]);

test('20th consecutive failure mutes the feed by setting disabled_at', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 19,
    ]);

    bindFetcherReturning(new Failed(new FeedFetchException('boom')));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe(20);
    expect($feed->disabled_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
});

test('failures below 20 do not mute the feed', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 18,
    ]);

    bindFetcherReturning(new Failed(new FeedFetchException('boom')));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe(19);
    expect($feed->disabled_at)->toBeNull();
});

test('Modified success clears consecutive_failures and disabled_at', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 20,
        'disabled_at' => Date::now()->subDay(),
    ]);

    bindFetcherReturning(new Modified(entries: [], etag: null, lastModified: null));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe(0);
    expect($feed->disabled_at)->toBeNull();
});

test('NotModified success clears consecutive_failures and disabled_at', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 20,
        'disabled_at' => Date::now()->subDay(),
    ]);

    bindFetcherReturning(new NotModified(etag: null, lastModified: null));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe(0);
    expect($feed->disabled_at)->toBeNull();
});

test('RateLimited honors Retry-After when larger than current backoff step', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 0,
    ]);

    bindFetcherReturning(new RateLimited(retryAfterSeconds: 600));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 12:10:00');
    expect($feed->consecutive_failures)->toBe(0);
    expect($feed->last_error)->toBe('HTTP 429 (retry after 600s)');
    expect($feed->disabled_at)->toBeNull();
});

test('RateLimited falls back to backoff floor when Retry-After is shorter', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 3,
    ]);

    bindFetcherReturning(new RateLimited(retryAfterSeconds: 10));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 13:00:00');
    expect($feed->consecutive_failures)->toBe(3);
});

test('Gone immediately mutes the feed without incrementing failures', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 2,
    ]);

    bindFetcherReturning(new Gone);

    runJob($feed->id);

    $feed->refresh();
    expect($feed->disabled_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->consecutive_failures)->toBe(2);
    expect($feed->last_error)->toBe('HTTP 410 Gone');
});

test('failed() hook records the same state as the Failed branch', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://example.com/rss',
        'consecutive_failures' => 0,
        'last_dispatched_at' => Date::now()->subMinute(),
    ]);

    (new RefreshFeedJob($feed->id))->failed(new RuntimeException('worker died'));

    $feed->refresh();
    expect($feed->consecutive_failures)->toBe(1);
    expect($feed->last_failed_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
    expect($feed->last_error)->toBe('worker died');
    expect($feed->last_dispatched_at)->toBeNull();
    expect($feed->next_check_at?->toDateTimeString())->toBe('2026-05-07 12:05:00');
});

test('failed() hook is a no-op when the feed has been deleted', function () {
    (new RefreshFeedJob(999_999))->failed(new RuntimeException('worker died'));

    expect(true)->toBeTrue();
});

test('feeds:retry-disabled dispatches a refresh job for each muted feed with subscribers', function () {
    Bus::fake();

    $muted = Feed::query()->create([
        'feed_url' => 'https://muted.example/rss',
        'disabled_at' => Date::now()->subWeek(),
    ]);
    $orphan = Feed::query()->create([
        'feed_url' => 'https://orphan.example/rss',
        'disabled_at' => Date::now()->subWeek(),
    ]);
    $healthy = Feed::query()->create([
        'feed_url' => 'https://healthy.example/rss',
    ]);
    makeFeedSubscriber($muted);
    makeFeedSubscriber($healthy);

    Artisan::call('feeds:retry-disabled');

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $muted->id);
    Bus::assertNotDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $orphan->id);
    Bus::assertNotDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $healthy->id);
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);
});

test('a successful retry on a muted feed silently re-enables it', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://recovered.example/rss',
        'disabled_at' => Date::now()->subWeek(),
        'consecutive_failures' => 20,
    ]);

    bindFetcherReturning(new Modified(entries: [], etag: null, lastModified: null));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->disabled_at)->toBeNull();
    expect($feed->consecutive_failures)->toBe(0);
});

test('a failed retry on a muted feed leaves it muted', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://still-broken.example/rss',
        'disabled_at' => Date::now()->subWeek(),
        'consecutive_failures' => 20,
    ]);

    bindFetcherReturning(new Failed(new FeedFetchException('still broken')));

    runJob($feed->id);

    $feed->refresh();
    expect($feed->disabled_at)->not->toBeNull();
    expect($feed->consecutive_failures)->toBe(21);
});

test('schedule registers feeds:retry-disabled daily at 03:00 with withoutOverlapping', function () {
    /** @var Schedule $schedule */
    $schedule = app(Schedule::class);

    $events = collect($schedule->events())->filter(
        fn (Event $event) => str_contains($event->command ?? '', 'feeds:retry-disabled'),
    );

    expect($events)->toHaveCount(1);

    /** @var Event $event */
    $event = $events->first();

    expect($event->expression)->toBe('0 3 * * *');
    expect($event->withoutOverlapping)->toBeTrue();
});
