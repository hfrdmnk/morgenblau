<?php

use App\Models\User;
use Illuminate\Contracts\Cache\LockTimeoutException;
use Illuminate\Http\Client\Response as HttpResponse;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Sleep;
use Mockery\MockInterface;
use Revolution\Bluesky\Client\AtpClient;

beforeEach(function () {
    Http::preventStrayRequests();
});

test('fresh access token in session goes straight to the pds with no refresh', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:freshtoken12345678abcd',
        'iss' => 'https://eurosky.social',
    ]);

    $this->actingAs($user);

    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => freshOAuthJwt(),
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss,
    ]);

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->never();

    fakeHtmlResponse();
    fakeListRecordsOnFactory($factory, []);

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk();
});

test('expired access token in session triggers exactly one refresh before the pds call', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:expiredtoken123456abcd',
        'iss' => 'https://eurosky.social',
    ]);

    $this->actingAs($user);

    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => expiredOAuthJwt(),
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss,
    ]);

    $factory = blueskyFactoryMock();
    // Mimic the package's real refresh behavior: rotate the in-session
    // access_token, so the second tokenExpired() check (inside the per-DID
    // lock) sees fresh tokens and the controller does not re-refresh.
    $factory->shouldReceive('refreshSession')
        ->once()
        ->andReturnUsing(function () use ($factory) {
            session()->put('bluesky_session.access_token', freshOAuthJwt());

            return $factory;
        });

    fakeHtmlResponse();
    fakeListRecordsOnFactory($factory, []);

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk();
});

test('lock held by another holder eventually times out instead of silently falling back to unauthenticated', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:lockedout12345678abcd',
        'iss' => 'https://eurosky.social',
    ]);

    $this->actingAs($user);

    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => expiredOAuthJwt(),
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss,
    ]);

    // Simulate another process holding the per-DID lock for the entire wait.
    $blocker = Cache::lock("bluesky:auth:{$user->did}", 30, owner: 'someone-else');
    expect($blocker->get())->toBeTrue();

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->never();

    fakeHtmlResponse();

    // Fake sleep so the 5s lock wait runs in microseconds; sync to Carbon so
    // the in-loop time check inside Lock::block() actually trips the timeout.
    Sleep::fake(syncWithCarbon: true);

    $this->withoutExceptionHandling();

    expect(fn () => $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com']))
        ->toThrow(LockTimeoutException::class);

    $blocker->forceRelease();
});

function fakeListRecordsOnFactory(MockInterface $factory, array $records = []): void
{
    $client = Mockery::mock(AtpClient::class);
    $client->shouldReceive('listRecords')
        ->andReturn(new HttpResponse(Http::response(['records' => $records], 200)->wait()));

    $factory->shouldReceive('client')->with(true)->andReturn($client);
}

function fakeHtmlResponse(): void
{
    Http::fake([
        'example.com' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" title="Main" href="/rss.xml"></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);
}
