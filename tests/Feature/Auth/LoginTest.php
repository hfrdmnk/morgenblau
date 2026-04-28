<?php

use App\Models\User;
use GuzzleHttp\Psr7\Response as Psr7Response;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Http\Client\RequestException;
use Illuminate\Http\Client\Response as HttpResponse;
use Tests\Concerns\FakesBlueskyOAuth;

uses(FakesBlueskyOAuth::class);

test('login page renders', function () {
    $this->get(route('login'))->assertOk();
});

test('POST /login redirects to Bluesky with handle hint', function () {
    $this->fakeBlueskyRedirect(hint: 'alice.bsky.social');

    $this->post(route('login'), ['handle' => 'alice.bsky.social'])
        ->assertRedirect('https://bsky.social/oauth/authorize');

    expect(session('atproto.hint'))->toBe('alice.bsky.social');
});

test('POST /login surfaces unreachable PDS as a handle validation error', function () {
    $this->fakeBlueskyRedirectThrows(
        new ConnectionException('cURL error 6: Could not resolve host')
    );

    $this->from(route('login'))
        ->post(route('login'), ['handle' => 'test.com'])
        ->assertRedirect(route('login'))
        ->assertSessionHasErrors(['handle']);
});

test('POST /login surfaces auth-server rejection as a handle validation error', function () {
    $this->fakeBlueskyRedirectThrows(
        new RequestException(new HttpResponse(
            new Psr7Response(400, [], '{"error":"invalid_request","error_description":"Invalid login_hint \"a\""}')
        ))
    );

    $this->from(route('login'))
        ->post(route('login'), ['handle' => 'a'])
        ->assertRedirect(route('login'))
        ->assertSessionHasErrors(['handle']);
});

test('POST /logout clears the Bluesky session and CSRF token', function () {
    $user = User::factory()->create();
    $previousToken = csrf_token();

    $this->actingAs($user)
        ->withSession([
            'atproto.handle' => 'alice.bsky.social',
            'bluesky_session' => ['did' => $user->did],
        ])
        ->post(route('logout'))
        ->assertRedirect('/');

    $this->assertGuest();
    expect(session('atproto.handle'))->toBeNull()
        ->and(session('bluesky_session'))->toBeNull()
        ->and(csrf_token())->not->toBe($previousToken);
});
