<?php

use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\Bus;
use Inertia\Testing\AssertableInertia as Assert;
use Revolution\Bluesky\Contracts\Factory as BlueskyFactory;

beforeEach(function () {
    Bus::fake();
    Carbon::setTestNow('2026-05-07 10:00:00');
});

afterEach(function () {
    Carbon::setTestNow();
});

test('user with no local subscriptions sees has_subscriptions=false without contacting PDS', function () {
    $user = User::factory()->create();

    $factory = Mockery::mock(BlueskyFactory::class);
    $factory->shouldReceive('withToken')->andReturnSelf();
    $factory->shouldReceive('refreshSession')->andReturnSelf();
    $factory->shouldNotReceive('client');
    app()->instance(BlueskyFactory::class, $factory);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page
            ->where('has_subscriptions', false)
            ->has('entries', 0));
});

test('user with a local subscription but zero entries sees has_subscriptions=true and empty entries', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://quiet.example/rss']);
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page
            ->where('has_subscriptions', true)
            ->has('entries', 0));
});
