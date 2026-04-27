<?php

use App\Models\User;
use GuzzleHttp\Psr7\Response as Psr7Response;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Http\Client\RequestException;
use Illuminate\Http\Client\Response as HttpResponse;
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

test('POST /login surfaces unreachable PDS as a handle validation error', function () {
    $provider = Mockery::mock(BlueskyProvider::class);
    $provider->shouldReceive('setScopes')->andReturnSelf();
    $provider->shouldReceive('hint')->andReturnSelf();
    $provider->shouldReceive('redirect')->andThrow(
        new ConnectionException('cURL error 6: Could not resolve host')
    );

    Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);

    $this->from(route('login'))
        ->post(route('login'), ['handle' => 'test.com'])
        ->assertRedirect(route('login'))
        ->assertSessionHasErrors(['handle']);
});

test('POST /login surfaces auth-server rejection as a handle validation error', function () {
    $provider = Mockery::mock(BlueskyProvider::class);
    $provider->shouldReceive('setScopes')->andReturnSelf();
    $provider->shouldReceive('hint')->andReturnSelf();
    $provider->shouldReceive('redirect')->andThrow(
        new RequestException(new HttpResponse(
            new Psr7Response(400, [], '{"error":"invalid_request","error_description":"Invalid login_hint \"a\""}')
        ))
    );

    Socialite::shouldReceive('driver')->with('bluesky')->andReturn($provider);

    $this->from(route('login'))
        ->post(route('login'), ['handle' => 'a'])
        ->assertRedirect(route('login'))
        ->assertSessionHasErrors(['handle']);
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
