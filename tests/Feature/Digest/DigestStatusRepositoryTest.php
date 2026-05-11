<?php

use App\Models\Feed;
use App\Models\FeedEntry;
use App\Models\Subscription;
use App\Models\User;
use App\Repositories\DigestStatusRepository;
use Carbon\CarbonImmutable;
use Illuminate\Support\Carbon;

beforeEach(function () {
    Carbon::setTestNow('2026-05-11 10:00:00');
});

afterEach(function () {
    Carbon::setTestNow();
});

function subscribeFeed(User $user, Feed $feed): Subscription
{
    return Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);
}

function newEntry(Feed $feed, array $overrides = []): FeedEntry
{
    return FeedEntry::query()->create(array_merge([
        'feed_id' => $feed->id,
        'guid' => 'urn:'.bin2hex(random_bytes(4)),
        'title' => 'Hi',
        'link' => null,
        'first_seen_at' => now(),
        'updated_at' => now(),
    ], $overrides));
}

test('returns a calm zero state when the user has no subscriptions', function () {
    $user = User::factory()->create();

    $status = app(DigestStatusRepository::class)->forUser(
        $user,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->pending)->toBeFalse()
        ->and($status->newCount)->toBe(0)
        ->and($status->latestEntryAt)->toBeNull();
});

test('pending is true when a subscribed feed was dispatched after since and not yet fetched', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => Carbon::parse('2026-05-11 09:30:00'),
        'last_fetched_at' => null,
    ]);
    subscribeFeed($user, $feed);

    $status = app(DigestStatusRepository::class)->forUser(
        $user,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->pending)->toBeTrue();
});

test('pending excludes feeds dispatched before since (cron-before-action)', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://cron.example/rss',
        'last_dispatched_at' => Carbon::parse('2026-05-11 08:00:00'),
        'last_fetched_at' => null,
    ]);
    subscribeFeed($user, $feed);

    $status = app(DigestStatusRepository::class)->forUser(
        $user,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->pending)->toBeFalse();
});

test('pending excludes feeds that have been fetched after dispatch', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://done.example/rss',
        'last_dispatched_at' => Carbon::parse('2026-05-11 09:30:00'),
        'last_fetched_at' => Carbon::parse('2026-05-11 09:31:00'),
    ]);
    subscribeFeed($user, $feed);

    $status = app(DigestStatusRepository::class)->forUser(
        $user,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->pending)->toBeFalse();
});

test('pending ignores feeds belonging to other users', function () {
    $me = User::factory()->create();
    $other = User::factory()->create();

    $theirs = Feed::query()->create([
        'feed_url' => 'https://theirs.example/rss',
        'last_dispatched_at' => Carbon::parse('2026-05-11 09:30:00'),
        'last_fetched_at' => null,
    ]);
    subscribeFeed($other, $theirs);

    $status = app(DigestStatusRepository::class)->forUser(
        $me,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->pending)->toBeFalse();
});

test('new_count counts entries first-seen after since for subscribed feeds', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    subscribeFeed($user, $feed);

    newEntry($feed, ['first_seen_at' => Carbon::parse('2026-05-11 09:30:00')]);
    newEntry($feed, ['first_seen_at' => Carbon::parse('2026-05-11 09:45:00')]);
    newEntry($feed, ['first_seen_at' => Carbon::parse('2026-05-11 08:30:00')]);

    $status = app(DigestStatusRepository::class)->forUser(
        $user,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->newCount)->toBe(2)
        ->and($status->latestEntryAt)->toBe(CarbonImmutable::parse('2026-05-11 09:45:00')->toIso8601String());
});

test('new_count does not count entries from feeds the user does not subscribe to', function () {
    $me = User::factory()->create();
    $other = User::factory()->create();

    $mine = Feed::query()->create(['feed_url' => 'https://mine.example/rss']);
    $theirs = Feed::query()->create(['feed_url' => 'https://theirs.example/rss']);
    subscribeFeed($me, $mine);
    subscribeFeed($other, $theirs);

    newEntry($mine, ['first_seen_at' => Carbon::parse('2026-05-11 09:30:00')]);
    newEntry($theirs, ['first_seen_at' => Carbon::parse('2026-05-11 09:45:00')]);

    $status = app(DigestStatusRepository::class)->forUser(
        $me,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->newCount)->toBe(1);
});

test('latestEntryAt is null when no new entries exist since', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://quiet.example/rss']);
    subscribeFeed($user, $feed);
    newEntry($feed, ['first_seen_at' => Carbon::parse('2026-05-11 08:00:00')]);

    $status = app(DigestStatusRepository::class)->forUser(
        $user,
        CarbonImmutable::parse('2026-05-11 09:00:00'),
    );

    expect($status->newCount)->toBe(0)
        ->and($status->latestEntryAt)->toBeNull();
});
