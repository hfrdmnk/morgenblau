<?php

use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use App\Services\Feeds\FeedRefreshScheduler;
use Illuminate\Contracts\Bus\Dispatcher as BusDispatcher;
use Illuminate\Support\Carbon;

beforeEach(function () {
    Carbon::setTestNow('2026-05-07 10:00:00');
});

afterEach(function () {
    Carbon::setTestNow();
});

function subscribeOne(Feed $feed): User
{
    $user = User::factory()->create();
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://x/'.$feed->id,
    ]);

    return $user;
}

test('a thrown dispatch leaves last_dispatched_at unstamped', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://broken-queue.example/rss',
        'last_dispatched_at' => null,
        'next_check_at' => null,
    ]);
    subscribeOne($feed);

    $busMock = Mockery::mock(BusDispatcher::class);
    $busMock->shouldReceive('dispatch')->andThrow(new RuntimeException('queue down'));
    $busMock->shouldReceive('dispatchToQueue')->andThrow(new RuntimeException('queue down'));
    $busMock->shouldReceive('dispatchSync')->andThrow(new RuntimeException('queue down'));
    $busMock->shouldReceive('dispatchNow')->andThrow(new RuntimeException('queue down'));
    app()->instance(BusDispatcher::class, $busMock);

    app(FeedRefreshScheduler::class)->dispatchAll();

    expect($feed->fresh()->last_dispatched_at)->toBeNull();
});

test('dispatchAll returns 0 when every dispatch throws', function () {
    $feed = Feed::query()->create([
        'feed_url' => 'https://broken-queue.example/rss',
    ]);
    subscribeOne($feed);

    $busMock = Mockery::mock(BusDispatcher::class);
    $busMock->shouldReceive('dispatch')->andThrow(new RuntimeException('queue down'));
    $busMock->shouldReceive('dispatchToQueue')->andThrow(new RuntimeException('queue down'));
    $busMock->shouldReceive('dispatchSync')->andThrow(new RuntimeException('queue down'));
    $busMock->shouldReceive('dispatchNow')->andThrow(new RuntimeException('queue down'));
    app()->instance(BusDispatcher::class, $busMock);

    $count = app(FeedRefreshScheduler::class)->dispatchAll();

    expect($count)->toBe(0);
});
