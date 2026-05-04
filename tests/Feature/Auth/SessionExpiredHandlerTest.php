<?php

use App\Enums\FlashToastType;
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

    $toast = session('inertia.flash_data')['toast'];
    expect($toast->type)->toBe(FlashToastType::Info);
    expect($toast->message)->toBe('Your session expired — please sign in again.');
});

test('does not stash url.intended for unsafe methods and redirects to login', function () {
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
        ->post(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertRedirect(route('login'));

    expect(session('url.intended'))->toBeNull();
    expect(auth()->check())->toBeFalse();
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
