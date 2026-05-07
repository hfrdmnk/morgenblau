<?php

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\Bus;

beforeEach(function () {
    Bus::fake();
    Carbon::setTestNow('2026-05-07 10:00:00');
});

afterEach(function () {
    Carbon::setTestNow();
});

function subscribeTo(User $user, Feed $feed): Subscription
{
    return Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);
}

test('stale feed is dispatched and surfaces in refreshing_feed_ids', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://stale.example/rss',
        'last_fetched_at' => now()->subMinutes(90),
        'last_dispatched_at' => null,
    ]);
    subscribeTo($user, $feed);
    $this->fakePdsList(['https://stale.example/rss']);

    $this->actingAs(freshenBluesky($user));

    $response = $this->get(route('consume'));

    $response->assertOk();

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $feed->id);

    $feed->refresh();
    expect($feed->last_dispatched_at)->not->toBeNull();
    expect($feed->last_dispatched_at->equalTo(now()))->toBeTrue();

    $ids = $response->viewData('page')['props']['refreshing_feed_ids'];
    expect($ids)->toBe([$feed->id]);
});

test('fresh feed does not dispatch and is absent from refreshing_feed_ids', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://fresh.example/rss',
        'last_fetched_at' => now()->subMinutes(10),
        'last_dispatched_at' => null,
    ]);
    subscribeTo($user, $feed);
    $this->fakePdsList(['https://fresh.example/rss']);

    $this->actingAs(freshenBluesky($user));

    $response = $this->get(route('consume'));

    Bus::assertNotDispatched(RefreshFeedJob::class);

    $feed->refresh();
    expect($feed->last_dispatched_at)->toBeNull();

    expect($response->viewData('page')['props']['refreshing_feed_ids'])->toBe([]);
});

test('never-fetched feed counts as stale and is dispatched', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://never.example/rss',
        'last_fetched_at' => null,
        'last_dispatched_at' => null,
    ]);
    subscribeTo($user, $feed);
    $this->fakePdsList(['https://never.example/rss']);

    $this->actingAs(freshenBluesky($user));

    $response = $this->get(route('consume'));

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $feed->id);

    $feed->refresh();
    expect($feed->last_dispatched_at?->equalTo(now()))->toBeTrue();
    expect($response->viewData('page')['props']['refreshing_feed_ids'])->toBe([$feed->id]);
});

test('in-flight feed is not re-dispatched but stays in refreshing_feed_ids', function () {
    $user = User::factory()->create();
    $dispatchedAt = now()->subMinutes(2);
    $feed = Feed::query()->create([
        'feed_url' => 'https://inflight.example/rss',
        'last_fetched_at' => null,
        'last_dispatched_at' => $dispatchedAt,
    ]);
    subscribeTo($user, $feed);
    $this->fakePdsList(['https://inflight.example/rss']);

    $this->actingAs(freshenBluesky($user));

    $response = $this->get(route('consume'));

    Bus::assertNotDispatched(RefreshFeedJob::class);

    $feed->refresh();
    expect($feed->last_dispatched_at->equalTo($dispatchedAt))->toBeTrue();

    expect($response->viewData('page')['props']['refreshing_feed_ids'])->toBe([$feed->id]);
});

test('feed dispatched longer than 5 minutes ago counts as not in flight and re-dispatches', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://expired.example/rss',
        'last_fetched_at' => null,
        'last_dispatched_at' => now()->subMinutes(10),
    ]);
    subscribeTo($user, $feed);
    $this->fakePdsList(['https://expired.example/rss']);

    $this->actingAs(freshenBluesky($user));

    $response = $this->get(route('consume'));

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $feed->id);

    $feed->refresh();
    expect($feed->last_dispatched_at->equalTo(now()))->toBeTrue();
    expect($response->viewData('page')['props']['refreshing_feed_ids'])->toBe([$feed->id]);
});

test('rapid re-renders within the in-flight window do not double-dispatch', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://rapid.example/rss',
        'last_fetched_at' => now()->subMinutes(90),
        'last_dispatched_at' => null,
    ]);
    subscribeTo($user, $feed);
    $this->fakePdsList(['https://rapid.example/rss']);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))->assertOk();
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);

    Carbon::setTestNow(now()->addMilliseconds(200));

    $this->get(route('consume'))->assertOk();

    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);
});

test('does not consider feeds belonging to other users', function () {
    $me = User::factory()->create();
    $other = User::factory()->create();

    $myFresh = Feed::query()->create([
        'feed_url' => 'https://mine.example/rss',
        'last_fetched_at' => now()->subMinutes(10),
        'last_dispatched_at' => null,
    ]);
    subscribeTo($me, $myFresh);

    $theirStale = Feed::query()->create([
        'feed_url' => 'https://theirs.example/rss',
        'last_fetched_at' => now()->subMinutes(90),
        'last_dispatched_at' => null,
    ]);
    subscribeTo($other, $theirStale);

    $this->fakePdsList(['https://mine.example/rss']);

    $this->actingAs(freshenBluesky($me));

    $response = $this->get(route('consume'));

    Bus::assertNotDispatched(RefreshFeedJob::class);
    expect($response->viewData('page')['props']['refreshing_feed_ids'])->toBe([]);

    $theirStale->refresh();
    expect($theirStale->last_dispatched_at)->toBeNull();
});
