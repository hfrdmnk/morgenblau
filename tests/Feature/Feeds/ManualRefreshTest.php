<?php

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use Illuminate\Cache\RateLimiter;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\Bus;
use Illuminate\Support\Facades\Cache;

beforeEach(function () {
    Bus::fake();
    Cache::flush();
    Carbon::setTestNow('2026-05-07 10:00:00');
});

afterEach(function () {
    Carbon::setTestNow();
});

function subscribe(User $user, Feed $feed): Subscription
{
    return Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://'.$user->did.'/app.skyreader.feed.subscription/'.$feed->id,
    ]);
}

test('guests are redirected to login', function () {
    $this->post(route('feeds.refresh'))
        ->assertRedirect(route('login'));

    Bus::assertNotDispatched(RefreshFeedJob::class);
});

test('dispatches RefreshFeedJob for the user feeds and stamps last_dispatched_at', function () {
    $user = freshenBluesky(User::factory()->create());

    $never = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => null,
    ]);
    $stale = Feed::query()->create([
        'feed_url' => 'https://b.example/rss',
        'last_dispatched_at' => Carbon::parse('2026-05-07 09:50:00'),
    ]);
    subscribe($user, $never);
    subscribe($user, $stale);

    $this->actingAs($user)
        ->from(route('consume'))
        ->post(route('feeds.refresh'))
        ->assertRedirect(route('consume'));

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $never->id);
    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $stale->id);
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 2);

    expect($never->fresh()->last_dispatched_at?->equalTo(now()))->toBeTrue();
    expect($stale->fresh()->last_dispatched_at?->equalTo(now()))->toBeTrue();
});

test('skips feeds currently in flight', function () {
    $user = freshenBluesky(User::factory()->create());

    $inFlightAt = Carbon::parse('2026-05-07 09:58:00');
    $inFlight = Feed::query()->create([
        'feed_url' => 'https://busy.example/rss',
        'last_dispatched_at' => $inFlightAt,
    ]);
    subscribe($user, $inFlight);

    $this->actingAs($user)->post(route('feeds.refresh'))->assertRedirect();

    Bus::assertNotDispatched(RefreshFeedJob::class);
    expect($inFlight->fresh()->last_dispatched_at?->equalTo($inFlightAt))->toBeTrue();
});

test('does not dispatch for feeds belonging to other users', function () {
    $other = User::factory()->create();
    $me = freshenBluesky(User::factory()->create());

    $mine = Feed::query()->create(['feed_url' => 'https://mine.example/rss']);
    $theirs = Feed::query()->create(['feed_url' => 'https://theirs.example/rss']);
    subscribe($me, $mine);
    subscribe($other, $theirs);

    $this->actingAs($me)->post(route('feeds.refresh'))->assertRedirect();

    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $mine->id);
    Bus::assertNotDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $theirs->id);
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);

    expect($theirs->fresh()->last_dispatched_at)->toBeNull();
});

test('the seventh request inside one minute is throttled', function () {
    $user = freshenBluesky(User::factory()->create());

    // Re-bind the rate limiter to a cache store that survives the
    // session/cookie flush between subsequent test requests.
    app()->bind(RateLimiter::class, fn () => new RateLimiter(Cache::store('array')));

    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    subscribe($user, $feed);

    $this->actingAs($user);

    for ($i = 0; $i < 6; $i++) {
        $this->post(route('feeds.refresh'))->assertRedirect();
    }

    $this->post(route('feeds.refresh'))->assertStatus(429);
});

test('runs cleanly when the user has no subscriptions', function () {
    $user = freshenBluesky(User::factory()->create());

    $this->actingAs($user)->post(route('feeds.refresh'))->assertRedirect();

    Bus::assertNotDispatched(RefreshFeedJob::class);
});

test('skips muted feeds even on manual refresh', function () {
    $user = freshenBluesky(User::factory()->create());

    $muted = Feed::query()->create([
        'feed_url' => 'https://muted.example/rss',
        'disabled_at' => Carbon::parse('2026-05-06 09:00:00'),
    ]);
    $healthy = Feed::query()->create([
        'feed_url' => 'https://healthy.example/rss',
    ]);
    subscribe($user, $muted);
    subscribe($user, $healthy);

    $this->actingAs($user)->post(route('feeds.refresh'))->assertRedirect();

    Bus::assertNotDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $muted->id);
    Bus::assertDispatched(RefreshFeedJob::class, fn ($job) => $job->feedId === $healthy->id);
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);

    expect($muted->fresh()->last_dispatched_at)->toBeNull();
});
