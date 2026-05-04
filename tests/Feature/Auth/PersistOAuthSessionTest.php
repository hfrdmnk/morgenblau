<?php

use App\Models\User;
use Illuminate\Support\Facades\Log;
use Revolution\Bluesky\Events\OAuthSessionUpdated;
use Revolution\Bluesky\Session\OAuthSession;

test('persists the rotated refresh token for the matching DID', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:rotateduser567890abcd',
        'refresh_token' => 'old-refresh',
        'iss' => 'https://eurosky.social',
    ]);

    $session = OAuthSession::create([
        'did' => $user->did,
        'refresh_token' => 'rotated-refresh',
        'iss' => 'https://eurosky.social',
        'access_token' => 'fresh-access',
    ]);

    event(new OAuthSessionUpdated($session));

    $user->refresh();
    expect($user->refresh_token)->toBe('rotated-refresh')
        ->and($user->iss)->toBe('https://eurosky.social')
        ->and(session('bluesky_session'))->toMatchArray($session->toArray());
});

test('records the first iss when an existing user has none', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:freshiss12345678901234',
        'refresh_token' => 'r',
        'iss' => null,
    ]);

    event(new OAuthSessionUpdated(OAuthSession::create([
        'did' => $user->did,
        'refresh_token' => 'rotated',
        'iss' => 'https://eurosky.social',
    ])));

    $user->refresh();
    expect($user->iss)->toBe('https://eurosky.social')
        ->and($user->refresh_token)->toBe('rotated');
});

test('rejects an iss change for the same DID and logs a warning', function () {
    Log::spy();

    $user = User::factory()->create([
        'did' => 'did:plc:isschange1234567890ab',
        'refresh_token' => 'old',
        'iss' => 'https://eurosky.social',
    ]);

    event(new OAuthSessionUpdated(OAuthSession::create([
        'did' => $user->did,
        'refresh_token' => 'rotated',
        'iss' => 'https://attacker.example',
        'access_token' => 'fresh',
    ])));

    $user->refresh();
    expect($user->iss)->toBe('https://eurosky.social')
        ->and($user->refresh_token)->toBe('rotated');

    Log::shouldHaveReceived('warning')
        ->withArgs(fn (string $msg, array $ctx) => $msg === 'oauth iss change rejected'
            && $ctx['did'] === $user->did
            && $ctx['old_iss'] === 'https://eurosky.social'
            && $ctx['new_iss'] === 'https://attacker.example')
        ->once();
});

test('listener is a no-op when the session has no DID', function () {
    User::factory()->create(['did' => 'did:plc:untouched1234567890abcd']);

    $session = OAuthSession::create(['refresh_token' => 'r']);

    event(new OAuthSessionUpdated($session));

    expect(User::find('did:plc:untouched1234567890abcd')->refresh_token)
        ->not->toBe('r');
});

test('syncs the rotated refresh token onto the currently-authenticated user instance', function () {
    // Without this, $request->user() within the same request keeps the
    // pre-rotation refresh_token, and the next $user->bluesky() call merges
    // that stale value back into the OAuthSession — which the response
    // middleware then writes back to DB+session, regressing the rotation.
    $user = User::factory()->create([
        'did' => 'did:plc:liveuser1234567890abcd',
        'refresh_token' => 'pre-rotation',
        'iss' => 'https://eurosky.social',
    ]);

    $this->actingAs($user);

    event(new OAuthSessionUpdated(OAuthSession::create([
        'did' => $user->did,
        'refresh_token' => 'post-rotation',
        'iss' => 'https://eurosky.social',
    ])));

    expect(auth()->user()->refresh_token)->toBe('post-rotation')
        ->and($user->refresh_token)->toBe('post-rotation');
});

test('does not touch the in-memory user when a different DID rotates', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:meuser123456789012abcd',
        'refresh_token' => 'mine',
    ]);

    $this->actingAs($user);

    User::factory()->create([
        'did' => 'did:plc:otheruser567890123abcd',
        'refresh_token' => 'theirs-old',
    ]);

    event(new OAuthSessionUpdated(OAuthSession::create([
        'did' => 'did:plc:otheruser567890123abcd',
        'refresh_token' => 'theirs-new',
    ])));

    expect(auth()->user()->refresh_token)->toBe('mine');
});
