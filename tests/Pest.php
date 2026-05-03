<?php

use Firebase\JWT\JWT;
use Illuminate\Foundation\Testing\LazilyRefreshDatabase;
use Tests\TestCase;

pest()->extend(TestCase::class)->use(LazilyRefreshDatabase::class)->in('Feature');

function freshOAuthJwt(int $expIn = 3600): string
{
    return oauthJwtWithExp(now()->addSeconds($expIn)->timestamp);
}

function expiredOAuthJwt(int $expiredFor = 60): string
{
    return oauthJwtWithExp(now()->subSeconds($expiredFor)->timestamp);
}

function oauthJwtWithExp(int $exp): string
{
    $header = JWT::urlsafeB64Encode((string) json_encode(['alg' => 'ES256', 'typ' => 'at+jwt']));
    $payload = JWT::urlsafeB64Encode((string) json_encode(['exp' => $exp]));

    return "{$header}.{$payload}.sig";
}
