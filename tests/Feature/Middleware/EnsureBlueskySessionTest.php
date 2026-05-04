<?php

use App\Models\User;
use Illuminate\Auth\AuthenticationException;

test('fresh access token passes through without refresh', function () {
    $user = userWithBlueskySession(accessToken: freshOAuthJwt());

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->never();

    $this->actingAs($user)->get(route('consume'))->assertOk();
});

test('expired access token triggers a refresh on page load', function () {
    $user = userWithBlueskySession(accessToken: expiredOAuthJwt());

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->once()->andReturnSelf();

    $this->actingAs($user)->get(route('consume'))->assertOk();
});

test('failed refresh on page load logs the user out and bounces to /login', function () {
    $user = userWithBlueskySession(accessToken: expiredOAuthJwt());

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->once()->andThrow(new AuthenticationException);

    $this->actingAs($user)
        ->get(route('consume'))
        ->assertRedirect(route('login'));

    $this->assertGuest();
});

test('logout route is exempted from the proactive check', function () {
    $user = userWithBlueskySession(accessToken: expiredOAuthJwt());

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->never();

    $this->actingAs($user)->post(route('logout'))->assertRedirect('/');
});

function userWithBlueskySession(string $accessToken): User
{
    $user = User::factory()->create([
        'iss' => 'https://eurosky.social',
    ]);

    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => $accessToken,
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss,
    ]);

    return $user;
}
