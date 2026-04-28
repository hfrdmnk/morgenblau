<?php

use App\Models\User;
use Revolution\Bluesky\Events\OAuthSessionUpdated;
use Revolution\Bluesky\Session\OAuthSession;

test('persists rotated refresh token + iss for the matching DID', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:rotateduser567890abcd',
        'refresh_token' => 'old-refresh',
        'iss' => 'https://old.example',
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

test('listener is a no-op when the session has no DID', function () {
    User::factory()->create(['did' => 'did:plc:untouched1234567890abcd']);

    $session = OAuthSession::create(['refresh_token' => 'r']);

    event(new OAuthSessionUpdated($session));

    expect(User::find('did:plc:untouched1234567890abcd')->refresh_token)
        ->not->toBe('r');
});
