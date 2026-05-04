<?php

use App\Models\User;
use Illuminate\Auth\AuthenticationException;
use Illuminate\Support\Facades\Log;

test('stashes url.intended after a refresh failure on a GET route', function () {
    Log::spy();

    $user = User::factory()->create();
    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => expiredOAuthJwt(),
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss,
    ]);

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->andThrow(new AuthenticationException);

    $this->actingAs($user)
        ->get(route('discover'))
        ->assertRedirect(route('login'));

    expect(session('url.intended'))->toBe(route('discover'));
});

test('does not stash url.intended for unsafe methods', function () {
    Log::spy();

    $user = User::factory()->create();
    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => expiredOAuthJwt(),
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss,
    ]);

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->andThrow(new AuthenticationException);

    $this->actingAs($user)
        ->post(route('subscriptions.discover'), ['url' => 'https://example.com']);

    expect(session('url.intended'))->toBeNull();
});

test('logs a warning with the request context on session expiry', function () {
    Log::spy();

    $user = User::factory()->create();
    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => expiredOAuthJwt(),
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss,
    ]);

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->andThrow(new AuthenticationException);

    $this->actingAs($user)->get(route('discover'));

    Log::shouldHaveReceived('warning')->withArgs(function (string $message, array $context) {
        return $message === 'oauth session ended'
            && $context['path'] === 'discover'
            && $context['method'] === 'GET'
            && $context['authed'] === true
            && $context['has_bluesky_session'] === true;
    })->once();
});
