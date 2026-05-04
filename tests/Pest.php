<?php

use App\Models\User;
use Firebase\JWT\JWT;
use Illuminate\Foundation\Testing\LazilyRefreshDatabase;
use Mockery\MockInterface;
use Revolution\Bluesky\Contracts\Factory as BlueskyFactory;
use Tests\Concerns\FakesBlueskyClient;
use Tests\TestCase;

pest()->extend(TestCase::class)
    ->use(LazilyRefreshDatabase::class, FakesBlueskyClient::class)
    ->in('Feature');

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

function blueskyFactoryMock(): MockInterface
{
    $factory = Mockery::mock(BlueskyFactory::class);
    $factory->shouldReceive('withToken')->andReturnSelf();

    app()->instance(BlueskyFactory::class, $factory);

    return $factory;
}

function freshenBluesky(User $user): User
{
    session()->put('bluesky_session', [
        'did' => $user->did,
        'access_token' => freshOAuthJwt(),
        'refresh_token' => $user->refresh_token,
        'iss' => $user->iss ?? 'https://bsky.social',
    ]);

    return $user;
}
