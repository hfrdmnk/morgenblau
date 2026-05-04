<?php

use App\Models\User;

test('guests visiting / see the welcome page', function () {
    $this->get('/')
        ->assertOk()
        ->assertInertia(fn ($page) => $page->component('welcome'));
});

test('authed users visiting / are redirected to /consume', function () {
    $user = freshenBluesky(User::factory()->create());

    $this->actingAs($user)
        ->get('/')
        ->assertRedirect(route('consume'));
});
