<?php

use App\Models\Feed;
use App\Models\FeedEntry;
use App\Models\Subscription;
use App\Models\User;
use Illuminate\Support\Facades\Bus;

beforeEach(function () {
    Bus::fake();
});

function makeFeedEntry(Feed $feed, array $overrides = []): FeedEntry
{
    return FeedEntry::query()->create(array_merge([
        'feed_id' => $feed->id,
        'guid' => 'urn:'.bin2hex(random_bytes(4)),
        'title' => 'Untitled',
        'link' => null,
        'summary' => null,
        'content' => null,
        'author' => null,
        'published_at' => null,
        'first_seen_at' => now(),
        'updated_at' => now(),
    ], $overrides));
}

test('renders the user\'s entries newest-first across feeds', function () {
    $user = User::factory()->create();
    $feedA = Feed::query()->create(['feed_url' => 'https://a.example/rss', 'title' => 'A']);
    $feedB = Feed::query()->create(['feed_url' => 'https://b.example/rss', 'title' => 'B']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feedA->id, 'at_uri' => 'at://x/a']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feedB->id, 'at_uri' => 'at://x/b']);
    $this->fakePdsList(['https://a.example/rss', 'https://b.example/rss']);

    makeFeedEntry($feedA, ['title' => 'Older A', 'published_at' => now()->subDays(2)]);
    makeFeedEntry($feedB, ['title' => 'Newer B', 'published_at' => now()->subHour()]);

    $this->actingAs(freshenBluesky($user));

    $response = $this->get(route('consume'));

    $response->assertOk();
    $entries = $response->viewData('page')['props']['entries'];

    expect($entries[0]['entry_title'])->toBe('Newer B');
    expect($entries[1]['entry_title'])->toBe('Older A');
});

test('caps entries at 200', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);
    $this->fakePdsList(['https://a.example/rss']);

    for ($i = 0; $i < 250; $i++) {
        makeFeedEntry($feed, ['guid' => "urn:{$i}", 'published_at' => now()->subMinutes($i)]);
    }

    $this->actingAs(freshenBluesky($user));

    $response = $this->get(route('consume'));

    expect($response->viewData('page')['props']['entries'])->toHaveCount(200);
});

test('orders by COALESCE(published_at, first_seen_at)', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);
    $this->fakePdsList(['https://a.example/rss']);

    makeFeedEntry($feed, ['title' => 'Has published_at', 'published_at' => now()->subDay(), 'first_seen_at' => now()->subYear()]);
    makeFeedEntry($feed, ['title' => 'Falls back', 'published_at' => null, 'first_seen_at' => now()->subHour()]);

    $this->actingAs(freshenBluesky($user));

    $entries = $this->get(route('consume'))->viewData('page')['props']['entries'];

    expect($entries[0]['entry_title'])->toBe('Falls back');
    expect($entries[1]['entry_title'])->toBe('Has published_at');
});

test('uses the resolved display title (custom_title beats pds_title beats feed.title beats feed_url)', function () {
    $user = User::factory()->create();

    $custom = Feed::query()->create(['feed_url' => 'https://custom.example/rss', 'title' => 'Feed title']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $custom->id, 'at_uri' => 'at://x/a', 'custom_title' => 'My nickname', 'pds_title' => 'PDS title']);
    makeFeedEntry($custom, ['title' => 'Custom row', 'published_at' => now()->subMinute()]);

    $pds = Feed::query()->create(['feed_url' => 'https://pds.example/rss', 'title' => 'Feed title']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $pds->id, 'at_uri' => 'at://x/b', 'pds_title' => 'PDS title']);
    makeFeedEntry($pds, ['title' => 'Pds row', 'published_at' => now()->subMinutes(2)]);

    $feedTitle = Feed::query()->create(['feed_url' => 'https://feed.example/rss', 'title' => 'Feed title']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feedTitle->id, 'at_uri' => 'at://x/c']);
    makeFeedEntry($feedTitle, ['title' => 'Feed-title row', 'published_at' => now()->subMinutes(3)]);

    $url = Feed::query()->create(['feed_url' => 'https://url.example/rss']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $url->id, 'at_uri' => 'at://x/d']);
    makeFeedEntry($url, ['title' => 'Url row', 'published_at' => now()->subMinutes(4)]);

    // Reconcile syncs custom_title / pds_title from the PDS records — mirror them here.
    $this->fakeListRecords($this->fakeBlueskyClient(), [
        ['feed_url' => 'https://custom.example/rss', 'title' => 'PDS title', 'custom_title' => 'My nickname'],
        ['feed_url' => 'https://pds.example/rss', 'title' => 'PDS title'],
        ['feed_url' => 'https://feed.example/rss'],
        ['feed_url' => 'https://url.example/rss'],
    ]);

    $this->actingAs(freshenBluesky($user));

    $entries = $this->get(route('consume'))->viewData('page')['props']['entries'];
    $byTitle = collect($entries)->keyBy('entry_title');

    expect($byTitle['Custom row']['display_title'])->toBe('My nickname');
    expect($byTitle['Pds row']['display_title'])->toBe('PDS title');
    expect($byTitle['Feed-title row']['display_title'])->toBe('Feed title');
    expect($byTitle['Url row']['display_title'])->toBe('https://url.example/rss');
});

test('does not render entries from other users\' subscriptions', function () {
    $me = User::factory()->create();
    $other = User::factory()->create();

    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    Subscription::query()->create(['user_id' => $other->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);
    makeFeedEntry($feed, ['title' => 'Theirs', 'published_at' => now()]);

    // The acting user has no PDS subscriptions.
    $this->fakePdsList([]);

    $this->actingAs(freshenBluesky($me));

    expect($this->get(route('consume'))->viewData('page')['props']['entries'])->toBeEmpty();
});
