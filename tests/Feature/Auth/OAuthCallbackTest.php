<?php

use App\Models\User;
use Illuminate\Support\Facades\Exceptions;
use Revolution\Bluesky\Session\OAuthSession;
use Tests\Concerns\FakesBlueskyOAuth;

uses(FakesBlueskyOAuth::class);

test('callback creates user, stashes handle, and logs in', function () {
    $session = OAuthSession::create([
        'did' => 'did:plc:testuser1234567890abcd',
        'handle' => 'alice.bsky.social',
        'iss' => 'https://bsky.social',
        'refresh_token' => 'fake-refresh-token',
    ]);
    $this->fakeBlueskyCallback($session);

    $this->withSession(['atproto.hint' => 'alice.bsky.social'])
        ->get(route('bluesky.oauth.redirect'))
        ->assertRedirect(route('consume'));

    $user = User::find('did:plc:testuser1234567890abcd');

    expect($user)->not->toBeNull()
        ->and($user->refresh_token)->toBe('fake-refresh-token')
        ->and($user->iss)->toBe('https://bsky.social');

    $this->assertAuthenticatedAs($user);
    expect(session('atproto.handle'))->toBe('alice.bsky.social');
});

test('callback updates the existing row when the same DID re-auths', function () {
    $existing = User::factory()->create([
        'did' => 'did:plc:testuser1234567890abcd',
        'refresh_token' => 'old-refresh-token',
        'iss' => 'https://old.example',
    ]);

    $session = OAuthSession::create([
        'did' => $existing->did,
        'handle' => 'alice.bsky.social',
        'iss' => 'https://eurosky.social',
        'refresh_token' => 'new-refresh-token',
    ]);
    $this->fakeBlueskyCallback($session);

    $this->get(route('bluesky.oauth.redirect'))
        ->assertRedirect(route('consume'));

    $existing->refresh();
    expect($existing->refresh_token)->toBe('new-refresh-token')
        ->and($existing->iss)->toBe('https://eurosky.social')
        ->and(User::count())->toBe(1);
});

test('callback redirects to the stashed intended URL when present', function () {
    $session = OAuthSession::create([
        'did' => 'did:plc:returnuser1234567890abcd',
        'handle' => 'alice.bsky.social',
        'iss' => 'https://bsky.social',
        'refresh_token' => 'fake-refresh-token',
    ]);
    $this->fakeBlueskyCallback($session);

    $this->withSession(['url.intended' => route('discover')])
        ->get(route('bluesky.oauth.redirect'))
        ->assertRedirect(route('discover'));
});

test('callback does not create a user or log anyone in when Socialite throws', function () {
    Exceptions::fake();
    $this->fakeBlueskyCallbackThrows(new RuntimeException('state mismatch'));

    $this->get(route('bluesky.oauth.redirect'));

    $this->assertGuest();
    expect(User::count())->toBe(0);
    Exceptions::assertReported(RuntimeException::class);
});
