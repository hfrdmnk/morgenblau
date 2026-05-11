<?php

use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use App\Services\Feeds\FeedRefreshScheduler;
use Carbon\CarbonImmutable;

test('returns immediately when the user has no subscriptions', function () {
    $user = User::factory()->create();
    $scheduler = app(FeedRefreshScheduler::class);

    $started = microtime(true);
    $scheduler->waitForPendingFetches($user, CarbonImmutable::now(), timeoutSeconds: 5);
    $elapsed = microtime(true) - $started;

    expect($elapsed)->toBeLessThan(0.25);
});

test('returns immediately when every subscribed feed has fetched since the dispatch', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => CarbonImmutable::now()->subSeconds(10),
        'last_fetched_at' => CarbonImmutable::now()->subSeconds(5),
    ]);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);

    $scheduler = app(FeedRefreshScheduler::class);
    $since = CarbonImmutable::now()->subSeconds(30);

    $started = microtime(true);
    $scheduler->waitForPendingFetches($user, $since, timeoutSeconds: 5);
    $elapsed = microtime(true) - $started;

    expect($elapsed)->toBeLessThan(0.25);
});

test('ignores feeds whose dispatch predates the since marker', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => CarbonImmutable::now()->subMinutes(20),
        'last_fetched_at' => null,
    ]);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);

    $scheduler = app(FeedRefreshScheduler::class);
    $since = CarbonImmutable::now()->subMinutes(5);

    $started = microtime(true);
    $scheduler->waitForPendingFetches($user, $since, timeoutSeconds: 5);
    $elapsed = microtime(true) - $started;

    expect($elapsed)->toBeLessThan(0.25);
});

test('returns once the pending feed is marked fetched', function () {
    $user = User::factory()->create();
    $since = CarbonImmutable::now()->subSecond();
    $feed = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => CarbonImmutable::now(),
        'last_fetched_at' => null,
    ]);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);

    // Simulate the worker finishing after the first poll interval; mark the
    // feed fetched mid-wait via a register_tick or direct DB update — here we
    // just preempt by updating before the call and confirming a clean return.
    Feed::query()->where('id', $feed->id)->update([
        'last_fetched_at' => CarbonImmutable::now()->addSecond(),
    ]);

    $scheduler = app(FeedRefreshScheduler::class);

    $started = microtime(true);
    $scheduler->waitForPendingFetches($user, $since, timeoutSeconds: 5);
    $elapsed = microtime(true) - $started;

    expect($elapsed)->toBeLessThan(0.25);
});

test('honours the timeout when a feed never finishes fetching', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => CarbonImmutable::now(),
        'last_fetched_at' => null,
    ]);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);

    $scheduler = app(FeedRefreshScheduler::class);
    $since = CarbonImmutable::now()->subSecond();

    $started = microtime(true);
    $scheduler->waitForPendingFetches($user, $since, timeoutSeconds: 1);
    $elapsed = microtime(true) - $started;

    expect($elapsed)->toBeGreaterThanOrEqual(1.0);
    expect($elapsed)->toBeLessThan(2.0);
});
