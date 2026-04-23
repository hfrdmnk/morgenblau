<?php

use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Laravel\Socialite\Facades\Socialite;
use Laravel\Socialite\Two\User as SocialiteUser;
use Revolution\Bluesky\Session\OAuthSession;
use Revolution\Bluesky\Socialite\BlueskyProvider;

uses(RefreshDatabase::class);

test('login page renders', function () {
    $this->get(route('login'))->assertOk();
});

test('POST /login redirects to Bluesky with handle hint', function () {
    $provider = Mockery::mock(BlueskyProvider::class);
    $provider->shouldReceive('setScopes')->andReturnSelf();
    $provider->shouldReceive('hint')->with('alice.bsky.social')->andReturnSelf();
    $provider->shouldReceive('redirect')->andReturn(
        redirect('https://bsky.social/oauth/authorize')
    );

    Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);

    $this->post(route('login'), ['handle' => 'alice.bsky.social'])
        ->assertRedirect('https://bsky.social/oauth/authorize');

    expect(session('atproto.hint'))->toBe('alice.bsky.social');
});

test('OAuth callback creates user, stashes handle, and logs in', function () {
    $session = OAuthSession::create([
        'did' => 'did:plc:testuser1234567890abcd',
        'handle' => 'alice.bsky.social',
        'iss' => 'https://bsky.social',
        'refresh_token' => 'fake-refresh-token',
    ]);

    $socialiteUser = new SocialiteUser;
    $socialiteUser->session = $session;

    $provider = Mockery::mock(BlueskyProvider::class);
    $provider->shouldReceive('setScopes')->andReturnSelf();
    $provider->shouldReceive('hint')->andReturnSelf();
    $provider->shouldReceive('user')->andReturn($socialiteUser);

    Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);

    $this->withSession(['atproto.hint' => 'alice.bsky.social'])
        ->get(route('bluesky.oauth.redirect'))
        ->assertRedirect(route('dashboard'));

    $user = User::find('did:plc:testuser1234567890abcd');

    expect($user)->not->toBeNull()
        ->and($user->refresh_token)->toBe('fake-refresh-token')
        ->and($user->iss)->toBe('https://bsky.social');

    $this->assertAuthenticatedAs($user);
    expect(session('atproto.handle'))->toBe('alice.bsky.social');
});

test('POST /logout ends the session', function () {
    $user = User::factory()->create();

    $this->actingAs($user)
        ->post(route('logout'))
        ->assertRedirect('/');

    $this->assertGuest();
});
