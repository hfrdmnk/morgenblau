<?php

use App\Models\User;
use Illuminate\Contracts\Cache\LockTimeoutException;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Sleep;

beforeEach(function () {
    Http::preventStrayRequests();
});

test('store blocks behind the per-user write lock and times out cleanly', function () {
    $user = User::factory()->create([
        'did' => 'did:plc:storelock1234567890ab',
    ]);

    $this->actingAs($user);

    // Simulate a concurrent batch holding the lock for the entire wait.
    $blocker = Cache::lock("subscriptions:write:{$user->did}", 30, owner: 'someone-else');
    expect($blocker->get())->toBeTrue();

    $this->fakeBlueskyClient();

    Sleep::fake(syncWithCarbon: true);

    $this->withoutExceptionHandling();

    expect(fn () => $this->post(route('subscriptions.store'), [
        'subscriptions' => [[
            'feed_url' => 'https://example.com/rss.xml',
            'title' => 'Foo',
            'site_url' => 'https://example.com',
            'source_type' => 'rss',
        ]],
    ]))->toThrow(LockTimeoutException::class);

    $blocker->forceRelease();
});
