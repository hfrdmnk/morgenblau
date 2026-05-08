<?php

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use App\Services\Subscriptions\SubscriptionService;
use Illuminate\Support\Facades\Bus;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Http;

beforeEach(function () {
    Http::preventStrayRequests();
    Cache::flush();
    Bus::fake();
});

test('adds new mirror rows and dispatches a refresh for unfetched feeds', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andReturn($this->fakeSuccessResponse([
        'records' => [
            [
                'uri' => 'at://did:plc:user/app.skyreader.feed.subscription/abc',
                'value' => [
                    'feedUrl' => 'https://example.com/rss',
                    'title' => 'Example',
                    'customTitle' => 'My example',
                ],
            ],
        ],
    ]));

    $user = User::factory()->create();

    app(SubscriptionService::class)->listSubscriptions($user);

    $feed = Feed::query()->where('feed_url', 'https://example.com/rss')->firstOrFail();
    $sub = Subscription::query()->where('user_id', $user->did)->firstOrFail();

    expect($sub->feed_id)->toBe($feed->id);
    expect($sub->at_uri)->toBe('at://did:plc:user/app.skyreader.feed.subscription/abc');
    expect($sub->custom_title)->toBe('My example');
    expect($sub->pds_title)->toBe('Example');
    expect($feed->fresh()->last_dispatched_at)->not->toBeNull();

    Bus::assertDispatched(RefreshFeedJob::class, fn (RefreshFeedJob $job) => $job->feedId === $feed->id);
});

test('removes mirror rows for vanished PDS subs', function () {
    $client = $this->fakeBlueskyClient();
    $user = User::factory()->create();

    $client->shouldReceive('listRecords')->andReturn(
        $this->fakeSuccessResponse([
            'records' => [
                ['uri' => 'at://did/app.skyreader.feed.subscription/a', 'value' => ['feedUrl' => 'https://a.example/rss']],
                ['uri' => 'at://did/app.skyreader.feed.subscription/b', 'value' => ['feedUrl' => 'https://b.example/rss']],
            ],
        ]),
        $this->fakeSuccessResponse([
            'records' => [
                ['uri' => 'at://did/app.skyreader.feed.subscription/a', 'value' => ['feedUrl' => 'https://a.example/rss']],
            ],
        ]),
    );

    $service = app(SubscriptionService::class);

    $service->listSubscriptions($user);
    expect(Subscription::query()->where('user_id', $user->did)->count())->toBe(2);

    Cache::flush();
    $service->listSubscriptions($user);

    $subs = Subscription::query()->where('user_id', $user->did)->get();
    expect($subs)->toHaveCount(1);
    expect($subs->first()->feed?->feed_url)->toBe('https://a.example/rss');
});

test('does not dispatch RefreshFeedJob for feeds already fetched or in flight', function () {
    Feed::query()->create([
        'feed_url' => 'https://fetched.example/rss',
        'last_fetched_at' => now()->subHour(),
    ]);
    Feed::query()->create([
        'feed_url' => 'https://inflight.example/rss',
        'last_dispatched_at' => now()->subMinute(),
    ]);

    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andReturn($this->fakeSuccessResponse([
        'records' => [
            ['uri' => 'at://did/a', 'value' => ['feedUrl' => 'https://fetched.example/rss']],
            ['uri' => 'at://did/b', 'value' => ['feedUrl' => 'https://inflight.example/rss']],
            ['uri' => 'at://did/c', 'value' => ['feedUrl' => 'https://new.example/rss']],
        ],
    ]));

    $user = User::factory()->create();
    app(SubscriptionService::class)->listSubscriptions($user);

    $newFeed = Feed::query()->where('feed_url', 'https://new.example/rss')->firstOrFail();

    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);
    Bus::assertDispatched(RefreshFeedJob::class, fn (RefreshFeedJob $job) => $job->feedId === $newFeed->id);
});

test('idempotent across repeated calls', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andReturn($this->fakeSuccessResponse([
        'records' => [
            ['uri' => 'at://did/a', 'value' => ['feedUrl' => 'https://example.com/rss', 'title' => 'Example']],
        ],
    ]));

    $user = User::factory()->create();
    $service = app(SubscriptionService::class);

    $service->listSubscriptions($user);
    Cache::flush();
    $service->listSubscriptions($user);

    expect(Subscription::query()->where('user_id', $user->did)->count())->toBe(1);
    Bus::assertDispatchedTimes(RefreshFeedJob::class, 1);
});

test('divergent at_uri for the same (user_id, feed_id) updates the row', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://example.com/rss']);
    Subscription::query()->create([
        'user_id' => $user->did,
        'feed_id' => $feed->id,
        'at_uri' => 'at://did/old-rkey',
    ]);

    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andReturn($this->fakeSuccessResponse([
        'records' => [
            ['uri' => 'at://did/new-rkey', 'value' => ['feedUrl' => 'https://example.com/rss']],
        ],
    ]));

    app(SubscriptionService::class)->listSubscriptions($user);

    $sub = Subscription::query()->where('user_id', $user->did)->firstOrFail();
    expect($sub->at_uri)->toBe('at://did/new-rkey');
    expect(Subscription::query()->where('user_id', $user->did)->count())->toBe(1);
});

test('user-A reconcile does not touch user-B subscription rows', function () {
    $userA = User::factory()->create();
    $userB = User::factory()->create();

    $shared = Feed::query()->create(['feed_url' => 'https://shared.example/rss']);
    $bOnly = Feed::query()->create(['feed_url' => 'https://b-only.example/rss']);

    Subscription::query()->create(['user_id' => $userB->did, 'feed_id' => $shared->id, 'at_uri' => 'at://b/shared']);
    $bOnlySub = Subscription::query()->create(['user_id' => $userB->did, 'feed_id' => $bOnly->id, 'at_uri' => 'at://b/only']);

    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andReturn($this->fakeSuccessResponse([
        'records' => [
            ['uri' => 'at://a/shared', 'value' => ['feedUrl' => 'https://shared.example/rss']],
        ],
    ]));

    app(SubscriptionService::class)->listSubscriptions($userA);

    expect(Subscription::query()->where('user_id', $userA->did)->count())->toBe(1);
    expect(Subscription::query()->where('user_id', $userB->did)->count())->toBe(2);
    expect(Subscription::query()->find($bOnlySub->id))->not->toBeNull();
});

test('a transient failure mid-reconcile rolls back partial mirror writes', function () {
    $user = User::factory()->create();

    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andReturn($this->fakeSuccessResponse([
        'records' => [
            ['uri' => 'at://did/a', 'value' => ['feedUrl' => 'https://a.example/rss']],
            ['uri' => 'at://did/b', 'value' => ['feedUrl' => 'https://b.example/rss']],
        ],
    ]));

    Feed::saving(function (Feed $feed): void {
        if ($feed->feed_url === 'https://b.example/rss') {
            throw new RuntimeException('mid-reconcile boom');
        }
    });

    try {
        try {
            app(SubscriptionService::class)->listSubscriptions($user);
        } catch (RuntimeException $e) {
            expect($e->getMessage())->toContain('mid-reconcile boom');
        }

        expect(Feed::query()->where('feed_url', 'https://a.example/rss')->exists())->toBeFalse();
        expect(Feed::query()->where('feed_url', 'https://b.example/rss')->exists())->toBeFalse();
        expect(Subscription::query()->where('user_id', $user->did)->exists())->toBeFalse();
    } finally {
        Feed::flushEventListeners();
    }
});
