<?php

use App\Models\Subscription;
use App\Models\User;
use Illuminate\Http\Client\Response as HttpResponse;
use Illuminate\Support\Facades\Http;
use Mockery\MockInterface;
use Revolution\Bluesky\Client\AtpClient;
use Revolution\Bluesky\Contracts\Factory as BlueskyFactory;

beforeEach(function () {
    Http::preventStrayRequests();
});

test('guests cannot add a subscription', function () {
    $this->post(route('subscriptions.store'), [
        'url' => 'https://overreacted.io',
    ])->assertRedirect(route('login'));
});

test('public subscription writes the record to the user PDS and skips the local DB', function () {
    Http::fake([
        'overreacted.io' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" href="/rss.xml"><title>overreacted</title></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $client = Mockery::mock(AtpClient::class);
    $client->shouldReceive('createRecord')
        ->once()
        ->withArgs(function (string $repo, string $collection, array $record) {
            expect($collection)->toBe('app.skyreader.feed.subscription');
            expect($record['feedUrl'])->toBe('https://overreacted.io/rss.xml');
            expect($record['category'])->toBe('source:blog');
            expect($record['sourceType'])->toBe('rss');

            return true;
        })
        ->andReturn(fakeSuccessResponse([
            'uri' => 'at://did:plc:test/app.skyreader.feed.subscription/abc',
            'cid' => 'bafy...',
        ]));

    bindBlueskyFactory($client);

    $user = User::factory()->create();
    $this->actingAs($user);

    $this->post(route('subscriptions.store'), [
        'url' => 'https://overreacted.io',
        'is_private' => false,
    ])->assertRedirect();

    expect(Subscription::count())->toBe(0);
});

test('private subscription writes a DB row and skips the PDS call', function () {
    Http::fake([
        'overreacted.io' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" href="https://overreacted.io/rss.xml"><title>overreacted</title></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $factory = Mockery::mock(BlueskyFactory::class);
    $factory->shouldNotReceive('withToken');
    app()->instance(BlueskyFactory::class, $factory);

    $user = User::factory()->create();
    $this->actingAs($user);

    $this->post(route('subscriptions.store'), [
        'url' => 'https://overreacted.io',
        'is_private' => true,
    ])->assertRedirect();

    $subscription = Subscription::sole();
    expect($subscription->user_did)->toBe($user->did);
    expect($subscription->feed_url)->toBe('https://overreacted.io/rss.xml');
    expect($subscription->category)->toBe('source:blog');
    expect($subscription->is_private)->toBeTrue();
});

test('an unresolvable URL returns a validation error', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><title>Plain page</title></head><body>Nothing here.</body></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $factory = Mockery::mock(BlueskyFactory::class);
    $factory->shouldNotReceive('withToken');
    app()->instance(BlueskyFactory::class, $factory);

    $user = User::factory()->create();
    $this->actingAs($user);

    $this->from(route('consume'))->post(route('subscriptions.store'), [
        'url' => 'https://example.com',
    ])
        ->assertRedirect(route('consume'))
        ->assertSessionHasErrors('url');

    expect(Subscription::count())->toBe(0);
});

test('an invalid URL returns a validation error', function () {
    $user = User::factory()->create();
    $this->actingAs($user);

    $this->from(route('consume'))->post(route('subscriptions.store'), [
        'url' => 'not-a-url',
    ])
        ->assertRedirect(route('consume'))
        ->assertSessionHasErrors('url');
});

function bindBlueskyFactory(MockInterface $client): void
{
    $factory = Mockery::mock(BlueskyFactory::class);
    $factory->shouldReceive('withToken')->andReturnSelf();
    $factory->shouldReceive('client')->with(true)->andReturn($client);

    app()->instance(BlueskyFactory::class, $factory);
}

function fakeSuccessResponse(array $body): HttpResponse
{
    return new HttpResponse(Http::response($body, 200)->wait());
}
