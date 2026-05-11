<?php

use App\Models\Feed;
use App\Models\FeedEntry;
use App\Models\Subscription;
use App\Models\User;
use Illuminate\Support\Carbon;

beforeEach(function () {
    Carbon::setTestNow('2026-05-11 10:00:00');
});

afterEach(function () {
    Carbon::setTestNow();
});

test('guests are unauthenticated', function () {
    $this->getJson(route('digest.status', ['since' => '2026-05-11T09:00:00Z']))
        ->assertUnauthorized();
});

test('returns the digest status payload for the authenticated user', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://a.example/rss',
        'last_dispatched_at' => Carbon::parse('2026-05-11 09:30:00'),
        'last_fetched_at' => null,
    ]);
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);

    FeedEntry::query()->create([
        'feed_id' => $feed->id,
        'guid' => 'urn:fresh',
        'title' => 'fresh',
        'first_seen_at' => Carbon::parse('2026-05-11 09:40:00'),
        'updated_at' => Carbon::parse('2026-05-11 09:40:00'),
    ]);

    $this->actingAs(freshenBluesky($user))
        ->getJson(route('digest.status', ['since' => '2026-05-11T09:00:00Z']))
        ->assertOk()
        ->assertJson([
            'pending' => true,
            'new_count' => 1,
        ])
        ->assertJsonPath('latest_entry_at', fn (?string $v) => is_string($v));
});

test('rejects a missing or invalid since with a 422', function () {
    $user = User::factory()->create();
    $this->actingAs(freshenBluesky($user));

    $this->getJson(route('digest.status'))
        ->assertStatus(422)
        ->assertJsonValidationErrors('since');

    $this->getJson(route('digest.status', ['since' => 'not-a-date']))
        ->assertStatus(422)
        ->assertJsonValidationErrors('since');
});

test('a user with no subscriptions sees a calm zero state', function () {
    $user = User::factory()->create();

    $this->actingAs(freshenBluesky($user))
        ->getJson(route('digest.status', ['since' => '2026-05-11T09:00:00Z']))
        ->assertOk()
        ->assertExactJson([
            'pending' => false,
            'new_count' => 0,
            'latest_entry_at' => null,
        ]);
});
