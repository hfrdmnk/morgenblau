<?php

use App\Models\User;

test('guest pages share null auth.user and auth.handle', function () {
    $this->get(route('login'))
        ->assertInertia(fn ($page) => $page
            ->where('auth.user', null)
            ->where('auth.handle', null)
        );
});

test('authed pages share the user model and the session-stashed handle', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:shareduser1234567890abcd',
    ]);

    $this->actingAs($user)
        ->withSession(['atproto.handle' => 'alice.bsky.social'])
        ->get(route('dashboard'))
        ->assertInertia(fn ($page) => $page
            ->where('auth.user.did', $user->did)
            ->where('auth.handle', 'alice.bsky.social')
        );
});
