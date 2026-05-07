<?php

use Illuminate\Auth\AuthenticationException;

test('fresh access token passes through without refresh', function () {
    $user = $this->userWithBlueskySession(accessToken: freshOAuthJwt());

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->never();
    $this->fakeListRecordsOnFactory($factory, []);

    $this->actingAs($user)->get(route('consume'))->assertOk();
});

test('expired access token triggers a refresh on page load', function () {
    $user = $this->userWithBlueskySession(accessToken: expiredOAuthJwt());

    $factory = blueskyFactoryMock();
    // Middleware refreshes once proactively; downstream PDS reads on the same
    // request may also call User::bluesky() and re-refresh because the mocked
    // refreshSession doesn't actually rotate the session token.
    $factory->shouldReceive('refreshSession')->atLeast()->once()->andReturnSelf();
    $this->fakeListRecordsOnFactory($factory, []);

    $this->actingAs($user)->get(route('consume'))->assertOk();
});

test('failed refresh on page load logs the user out and bounces to /login', function () {
    $user = $this->userWithBlueskySession(accessToken: expiredOAuthJwt());

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->once()->andThrow(new AuthenticationException);

    $this->actingAs($user)
        ->get(route('consume'))
        ->assertRedirect(route('login'));

    $this->assertGuest();
});

test('logout route is exempted from the proactive check', function () {
    $user = $this->userWithBlueskySession(accessToken: expiredOAuthJwt());

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')->never();

    $this->actingAs($user)->post(route('logout'))->assertRedirect('/');
});
