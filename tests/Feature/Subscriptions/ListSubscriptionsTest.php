<?php

use App\Exceptions\PdsReadException;
use App\Models\User;
use App\Services\Subscriptions\SubscriptionService;
use Illuminate\Support\Facades\Bus;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Facades\Log;

beforeEach(function () {
    Http::preventStrayRequests();
    Cache::flush();
    Bus::fake();
});

test('paginates through cursor pages until the cursor is empty', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')
        ->times(3)
        ->andReturn(
            $this->fakeSuccessResponse([
                'records' => [['value' => ['feedUrl' => 'https://a.example/rss']]],
                'cursor' => 'page2',
            ]),
            $this->fakeSuccessResponse([
                'records' => [['value' => ['feedUrl' => 'https://b.example/rss']]],
                'cursor' => 'page3',
            ]),
            $this->fakeSuccessResponse([
                'records' => [['value' => ['feedUrl' => 'https://c.example/rss']]],
            ]),
        );

    $user = User::factory()->create();
    $subs = app(SubscriptionService::class)->listSubscriptions($user);

    expect($subs)->toHaveCount(3)
        ->and($subs->toCollection()->pluck('feedUrl')->all())
        ->toBe([
            'https://a.example/rss',
            'https://b.example/rss',
            'https://c.example/rss',
        ]);
});

test('returns an empty collection when the user has no subscriptions', function () {
    $client = $this->fakeBlueskyClient();
    $this->fakeListRecords($client, []);

    expect(app(SubscriptionService::class)->listSubscriptions(User::factory()->create()))
        ->toHaveCount(0);
});

test('throws PdsReadException when the PDS responds with 5xx', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')
        ->andReturn($this->fakeFailureResponse(503, ['error' => 'Unavailable']));

    expect(fn () => app(SubscriptionService::class)->listSubscriptions(User::factory()->create()))
        ->toThrow(PdsReadException::class);
});

test('caps pagination and stops after the safety limit', function () {
    Log::spy();

    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andReturn(
        $this->fakeSuccessResponse([
            'records' => [['value' => ['feedUrl' => 'https://infinite.example/rss']]],
            'cursor' => 'never-empty',
        ]),
    );

    $subs = app(SubscriptionService::class)->listSubscriptions(User::factory()->create());

    expect($subs->count())->toBeLessThanOrEqual(50);
    Log::shouldHaveReceived('warning')
        ->withArgs(fn (string $msg) => str_contains($msg, 'pagination cap'))
        ->once();
});

test('skips records missing a feedUrl rather than failing the whole call', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')
        ->andReturn($this->fakeSuccessResponse([
            'records' => [
                ['value' => ['feedUrl' => 'https://valid.example/rss']],
                ['value' => ['title' => 'orphan']],
                ['value' => ['feedUrl' => '']],
            ],
        ]));

    $subs = app(SubscriptionService::class)->listSubscriptions(User::factory()->create());

    expect($subs)->toHaveCount(1)
        ->and($subs->toCollection()->first()->feedUrl)->toBe('https://valid.example/rss');
});

test('caches the result between calls within the TTL', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->once()->andReturn(
        $this->fakeSuccessResponse([
            'records' => [['value' => ['feedUrl' => 'https://cached.example/rss']]],
        ]),
    );

    $service = app(SubscriptionService::class);
    $user = User::factory()->create();

    $service->listSubscriptions($user);
    $service->listSubscriptions($user);
});
