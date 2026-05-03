<?php

use App\Models\User;
use Revolution\Bluesky\Session\OAuthSession;

beforeEach(function () {
    $this->did = 'did:plc:abc123abc123abc123abc123';
});

test('tokenForBluesky merges session bluesky_session over db fields when dids match', function () {
    $user = User::factory()->create([
        'did' => $this->did,
        'refresh_token' => 'db-refresh',
        'iss' => 'https://eurosky.social',
    ]);

    session()->put('bluesky_session', [
        'did' => $this->did,
        'access_token' => freshOAuthJwt(),
        'refresh_token' => 'db-refresh',
        'iss' => 'https://eurosky.social',
    ]);

    $session = invadeTokenForBluesky($user);

    expect($session->token())->not->toBe('')
        ->and($session->tokenExpired())->toBeFalse();
});

test('tokenForBluesky ignores session payload from a different did', function () {
    $user = User::factory()->create([
        'did' => $this->did,
        'refresh_token' => 'db-refresh',
    ]);

    session()->put('bluesky_session', [
        'did' => 'did:plc:somebody-else-entirely',
        'access_token' => freshOAuthJwt(),
    ]);

    $session = invadeTokenForBluesky($user);

    expect($session->token())->toBe('')
        ->and($session->tokenExpired())->toBeTrue()
        ->and($session->refresh())->toBe('db-refresh');
});

test('tokenForBluesky returns db-only session when laravel session has no bluesky_session', function () {
    $user = User::factory()->create([
        'did' => $this->did,
        'refresh_token' => 'db-refresh',
        'iss' => 'https://eurosky.social',
    ]);

    expect(session('bluesky_session'))->toBeNull();

    $session = invadeTokenForBluesky($user);

    expect($session->did())->toBe($this->did)
        ->and($session->refresh())->toBe('db-refresh')
        ->and($session->issuer())->toBe('https://eurosky.social')
        ->and($session->tokenExpired())->toBeTrue();
});

test('tokenForBluesky lets the db refresh_token win over a stale cached one', function () {
    $user = User::factory()->create([
        'did' => $this->did,
        'refresh_token' => 'rotated-in-db',
    ]);

    session()->put('bluesky_session', [
        'did' => $this->did,
        'refresh_token' => 'stale-in-session',
        'access_token' => freshOAuthJwt(),
    ]);

    $session = invadeTokenForBluesky($user);

    expect($session->refresh())->toBe('rotated-in-db');
});

function invadeTokenForBluesky(User $user): OAuthSession
{
    $reflection = new ReflectionMethod($user, 'tokenForBluesky');

    return $reflection->invoke($user);
}
