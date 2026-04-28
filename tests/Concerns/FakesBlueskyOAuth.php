<?php

namespace Tests\Concerns;

use Laravel\Socialite\Facades\Socialite;
use Laravel\Socialite\Two\User as SocialiteUser;
use Mockery;
use Revolution\Bluesky\Session\OAuthSession;
use Revolution\Bluesky\Socialite\BlueskyProvider;
use Throwable;

trait FakesBlueskyOAuth
{
    protected function fakeBlueskyRedirect(?string $hint = null, string $redirectUrl = 'https://bsky.social/oauth/authorize'): void
    {
        $provider = Mockery::mock(BlueskyProvider::class);
        $provider->shouldReceive('setScopes')->andReturnSelf();
        $hintExpectation = $provider->shouldReceive('hint');
        if ($hint !== null) {
            $hintExpectation->with($hint);
        }
        $hintExpectation->andReturnSelf();
        $provider->shouldReceive('redirect')->andReturn(redirect($redirectUrl));

        Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);
    }

    protected function fakeBlueskyRedirectThrows(Throwable $error): void
    {
        $provider = Mockery::mock(BlueskyProvider::class);
        $provider->shouldReceive('setScopes')->andReturnSelf();
        $provider->shouldReceive('hint')->andReturnSelf();
        $provider->shouldReceive('redirect')->andThrow($error);

        Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);
    }

    protected function fakeBlueskyCallback(OAuthSession $session): void
    {
        $socialiteUser = new SocialiteUser;
        $socialiteUser->session = $session;

        $provider = Mockery::mock(BlueskyProvider::class);
        $provider->shouldReceive('setScopes')->andReturnSelf();
        $provider->shouldReceive('hint')->andReturnSelf();
        $provider->shouldReceive('user')->andReturn($socialiteUser);

        Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);
    }

    protected function fakeBlueskyCallbackThrows(Throwable $error): void
    {
        $provider = Mockery::mock(BlueskyProvider::class);
        $provider->shouldReceive('setScopes')->andReturnSelf();
        $provider->shouldReceive('hint')->andReturnSelf();
        $provider->shouldReceive('user')->andThrow($error);

        Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);
    }
}
