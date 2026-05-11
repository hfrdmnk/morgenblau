<?php

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Services\Feeds\FeedJobDispatcher;
use Illuminate\Contracts\Bus\Dispatcher as BusDispatcher;
use Illuminate\Support\Facades\Bus;

function brokenBus(): void
{
    $bus = Mockery::mock(BusDispatcher::class);
    $bus->shouldReceive('dispatch')->andThrow(new RuntimeException('queue down'));
    $bus->shouldReceive('dispatchToQueue')->andThrow(new RuntimeException('queue down'));
    $bus->shouldReceive('dispatchSync')->andThrow(new RuntimeException('queue down'));
    $bus->shouldReceive('dispatchNow')->andThrow(new RuntimeException('queue down'));
    app()->instance(BusDispatcher::class, $bus);
}

test('happy path stamps last_dispatched_at and queues the job', function () {
    Bus::fake();
    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss', 'last_dispatched_at' => null]);

    $result = app(FeedJobDispatcher::class)->dispatch($feed->id);

    expect($result)->toBeTrue();
    expect($feed->fresh()->last_dispatched_at)->not->toBeNull();
    Bus::assertDispatched(RefreshFeedJob::class);
});

test('a thrown dispatch rolls back the stamp', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://broken.example/rss', 'last_dispatched_at' => null]);
    brokenBus();

    $result = app(FeedJobDispatcher::class)->dispatch($feed->id);

    expect($result)->toBeFalse();
    expect($feed->fresh()->last_dispatched_at)->toBeNull();
});

test('a failure on one feed does not stamp later feeds', function () {
    $feed1 = Feed::query()->create(['feed_url' => 'https://a.example/rss', 'last_dispatched_at' => null]);
    $feed2 = Feed::query()->create(['feed_url' => 'https://b.example/rss', 'last_dispatched_at' => null]);

    Bus::fake();
    $dispatcher = app(FeedJobDispatcher::class);

    $result1 = $dispatcher->dispatch($feed1->id);
    expect($result1)->toBeTrue();
    expect($feed1->fresh()->last_dispatched_at)->not->toBeNull();

    brokenBus();
    $result2 = $dispatcher->dispatch($feed2->id);

    expect($result2)->toBeFalse();
    expect($feed2->fresh()->last_dispatched_at)->toBeNull();
});
