<?php

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

test('user with zero PDS subscriptions sees has_subscriptions=false', function () {
    $user = User::factory()->create();
    $this->fakeListRecords($this->fakeBlueskyClient(), []);

    $this->actingAs(freshenBluesky($user));

    $props = $this->get(route('consume'))->viewData('page')['props'];

    expect($props['has_subscriptions'])->toBeFalse();
    expect($props['entries'])->toBe([]);
    expect($props['refreshing_feed_ids'])->toBe([]);
});

test('first visit with PDS subscriptions but empty local mirror reconciles and reports has_subscriptions=true', function () {
    $user = User::factory()->create();
    $this->fakeListRecords($this->fakeBlueskyClient(), [
        ['feed_url' => 'https://pds-only.example/rss'],
    ]);

    expect(Subscription::query()->where('user_id', $user->did)->exists())->toBeFalse();

    $this->actingAs(freshenBluesky($user));

    $props = $this->get(route('consume'))->viewData('page')['props'];

    expect($props['has_subscriptions'])->toBeTrue();
    expect($props['entries'])->toBe([]);
    // Reconcile created the feed + subscription rows.
    expect(Subscription::query()->where('user_id', $user->did)->count())->toBe(1);
});

test('user with subscriptions, no entries, and an in-flight feed sees has_subscriptions=true with refreshing_feed_ids populated', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://inflight.example/rss',
        'last_fetched_at' => null,
        'last_dispatched_at' => now()->subMinutes(2),
    ]);
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);
    $this->fakeListRecords($this->fakeBlueskyClient(), [
        ['feed_url' => 'https://inflight.example/rss'],
    ]);

    $this->actingAs(freshenBluesky($user));

    $props = $this->get(route('consume'))->viewData('page')['props'];

    expect($props['has_subscriptions'])->toBeTrue();
    expect($props['entries'])->toBe([]);
    expect($props['refreshing_feed_ids'])->toBe([$feed->id]);
});

test('user with subscriptions, no entries, and nothing in flight sees has_subscriptions=true with empty refreshing_feed_ids', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://quiet.example/rss',
        'last_fetched_at' => now()->subMinutes(10),
        'last_dispatched_at' => null,
    ]);
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);
    $this->fakeListRecords($this->fakeBlueskyClient(), [
        ['feed_url' => 'https://quiet.example/rss'],
    ]);

    $this->actingAs(freshenBluesky($user));

    $props = $this->get(route('consume'))->viewData('page')['props'];

    expect($props['has_subscriptions'])->toBeTrue();
    expect($props['entries'])->toBe([]);
    expect($props['refreshing_feed_ids'])->toBe([]);
});

test('local subscriptions for feeds removed from PDS are reconciled away and has_subscriptions=false', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://stale-mirror.example/rss']);
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);

    // PDS now reports zero subscriptions; reconcile should delete the local mirror row.
    $this->fakeListRecords($this->fakeBlueskyClient(), []);

    $this->actingAs(freshenBluesky($user));

    $props = $this->get(route('consume'))->viewData('page')['props'];

    expect($props['has_subscriptions'])->toBeFalse();
    expect(Subscription::query()->where('user_id', $user->did)->exists())->toBeFalse();
});
