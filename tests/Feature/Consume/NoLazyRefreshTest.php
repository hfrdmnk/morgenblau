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

test('visiting the digest does not dispatch any RefreshFeedJob', function () {
    $user = User::factory()->create();

    $stale = Feed::query()->create([
        'feed_url' => 'https://stale.example/rss',
        'last_fetched_at' => now()->subDays(2),
        'last_dispatched_at' => null,
    ]);
    $never = Feed::query()->create([
        'feed_url' => 'https://never.example/rss',
        'last_fetched_at' => null,
        'last_dispatched_at' => null,
    ]);

    foreach ([$stale, $never] as $feed) {
        Subscription::query()->create([
            'user_id' => $user->did,
            'feed_id' => $feed->id,
            'at_uri' => 'at://x/'.$feed->id,
        ]);
    }

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))->assertOk();

    Bus::assertNotDispatched(RefreshFeedJob::class);

    expect($stale->fresh()->last_dispatched_at)->toBeNull();
    expect($never->fresh()->last_dispatched_at)->toBeNull();
});
