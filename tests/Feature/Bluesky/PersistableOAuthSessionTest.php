<?php

use App\Bluesky\PersistableOAuthSession;

test('treats the token as fresh when expires_at is comfortably in the future', function () {
    $session = PersistableOAuthSession::create([
        'access_token' => 'opaque-not-a-jwt',
        'expires_at' => now()->addMinutes(30)->getTimestamp(),
    ]);

    expect($session->tokenExpired())->toBeFalse();
});

test('treats the token as expired once expires_at has passed', function () {
    $session = PersistableOAuthSession::create([
        'access_token' => 'opaque-not-a-jwt',
        'expires_at' => now()->subSeconds(5)->getTimestamp(),
    ]);

    expect($session->tokenExpired())->toBeTrue();
});

test('treats the token as expired inside the 30s safety buffer', function () {
    $session = PersistableOAuthSession::create([
        'access_token' => 'opaque-not-a-jwt',
        'expires_at' => now()->addSeconds(10)->getTimestamp(),
    ]);

    expect($session->tokenExpired())->toBeTrue();
});

test('falls back to the JWT exp claim when expires_at is missing', function () {
    $freshJwt = freshOAuthJwt(3600);
    $expiredJwt = expiredOAuthJwt();

    expect(PersistableOAuthSession::create(['access_token' => $freshJwt])->tokenExpired())->toBeFalse()
        ->and(PersistableOAuthSession::create(['access_token' => $expiredJwt])->tokenExpired())->toBeTrue();
});

test('treats an opaque token as expired when neither expires_at nor JWT exp is parseable', function () {
    $session = PersistableOAuthSession::create([
        'access_token' => 'opaque-not-a-jwt',
    ]);

    expect($session->tokenExpired())->toBeTrue();
});

test('accepts numeric-string expires_at values', function () {
    $session = PersistableOAuthSession::create([
        'access_token' => 'opaque',
        'expires_at' => (string) now()->addMinutes(10)->getTimestamp(),
    ]);

    expect($session->tokenExpired())->toBeFalse();
});
