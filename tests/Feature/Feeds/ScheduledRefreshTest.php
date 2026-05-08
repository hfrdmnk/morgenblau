<?php

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use Illuminate\Console\Scheduling\Event;
use Illuminate\Console\Scheduling\Schedule;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\Artisan;
use Illuminate\Support\Facades\Bus;

beforeEach(function () {
    Bus::fake();
    Carbon::setTestNow('2026-05-07 10:00:00');
});

afterEach(function () {
    Carbon::setTestNow();
});

function makeSubscriber(Feed $feed): User
{
    $user = User::factory()->create();
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);

    return $user;
}

test('dispatches RefreshFeedJob for every feed with at least one subscriber', function () {
    $subscribed1 = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    $subscribed2 = Feed::query()->create(['feed_url' => 'https://b.example/rss']);
    $orphan = Feed::query()->create(['feed_url' => 'https://orphan.example/rss']);

    makeSubscriber($subscribed1);
    makeSubscriber($subscribed2);

    Artisan::call('feeds:refresh-all');

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $subscribed1->id);
    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $subscribed2->id);
    Bus::assertNotDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $orphan->id);
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 2);
});

test('only dispatches once per feed even when multiple users subscribe to it', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://shared.example/rss']);
    makeSubscriber($feed);
    makeSubscriber($feed);
    makeSubscriber($feed);

    Artisan::call('feeds:refresh-all');

    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);
});

test('sets last_dispatched_at = now() on every dispatched feed', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => null,
    ]);
    makeSubscriber($feed);

    Artisan::call('feeds:refresh-all');

    $feed->refresh();
    expect($feed->last_dispatched_at)->not->toBeNull();
    expect($feed->last_dispatched_at->equalTo(now()))->toBeTrue();
});

test('does not touch last_dispatched_at on orphan feeds', function () {
    $original = Carbon::parse('2025-01-01 00:00:00');
    $orphan = Feed::query()->create([
        'feed_url' => 'https://orphan.example/rss',
        'last_dispatched_at' => $original,
    ]);

    Artisan::call('feeds:refresh-all');

    $orphan->refresh();
    expect($orphan->last_dispatched_at?->equalTo($original))->toBeTrue();
});

test('skips feeds whose next_check_at is still in the future', function () {
    $future = Feed::query()->create([
        'feed_url' => 'https://future.example/rss',
        'next_check_at' => Carbon::parse('2026-05-07 10:15:00'),
    ]);
    makeSubscriber($future);

    Artisan::call('feeds:refresh-all');

    Bus::assertNotDispatched(RefreshFeedJob::class);
    expect($future->fresh()->last_dispatched_at)->toBeNull();
});

test('dispatches feeds whose next_check_at has elapsed or is null', function () {
    $elapsed = Feed::query()->create([
        'feed_url' => 'https://elapsed.example/rss',
        'next_check_at' => Carbon::parse('2026-05-07 09:30:00'),
    ]);
    $never = Feed::query()->create([
        'feed_url' => 'https://never.example/rss',
        'next_check_at' => null,
    ]);
    makeSubscriber($elapsed);
    makeSubscriber($never);

    Artisan::call('feeds:refresh-all');

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $elapsed->id);
    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $never->id);
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 2);
});

test('runs cleanly with zero feeds', function () {
    Artisan::call('feeds:refresh-all');

    Bus::assertNotDispatched(RefreshFeedJob::class);
});

test('schedule registers feeds:refresh-all every thirty minutes with withoutOverlapping', function () {
    /** @var Schedule $schedule */
    $schedule = app(Schedule::class);

    $events = collect($schedule->events())->filter(
        fn (Event $event) => str_contains($event->command ?? '', 'feeds:refresh-all'),
    );

    expect($events)->toHaveCount(1);

    /** @var Event $event */
    $event = $events->first();

    expect($event->expression)->toBe('*/30 * * * *');
    expect($event->withoutOverlapping)->toBeTrue();
});
