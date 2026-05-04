<?php

use App\Models\User;

test('guests are redirected to the login page', function () {
    $response = $this->get(route('consume'));
    $response->assertRedirect(route('login'));
});

test('authenticated users can visit consume', function () {
    $user = freshenBluesky(User::factory()->create());
    $this->actingAs($user);

    $response = $this->get(route('consume'));
    $response->assertOk();
});
