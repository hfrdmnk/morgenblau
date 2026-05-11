<?php

use App\Models\Feed;
use App\Models\FeedEntry;
use App\Models\Subscription;
use App\Models\User;
use Illuminate\Support\Facades\Bus;
use Inertia\Testing\AssertableInertia as Assert;

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

    makeFeedEntry($feedA, ['title' => 'Older A', 'published_at' => now()->subDays(2)]);
    makeFeedEntry($feedB, ['title' => 'Newer B', 'published_at' => now()->subHour()]);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertOk()
        ->assertInertia(fn (Assert $page) => $page
            ->component('consume')
            ->where('has_subscriptions', true)
            ->loadDeferredProps(fn (Assert $loaded) => $loaded
                ->has('entries', 2)
                ->where('entries.0.entry_title', 'Newer B')
                ->where('entries.1.entry_title', 'Older A')));
});

test('caps entries at 200', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);

    for ($i = 0; $i < 250; $i++) {
        makeFeedEntry($feed, ['guid' => "urn:{$i}", 'published_at' => now()->subMinutes($i)]);
    }

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page->loadDeferredProps(fn (Assert $loaded) => $loaded->has('entries', 200)));
});

test('orders by COALESCE(published_at, first_seen_at)', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);

    makeFeedEntry($feed, ['title' => 'Has published_at', 'published_at' => now()->subDay(), 'first_seen_at' => now()->subYear()]);
    makeFeedEntry($feed, ['title' => 'Falls back', 'published_at' => null, 'first_seen_at' => now()->subHour()]);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page->loadDeferredProps(fn (Assert $loaded) => $loaded
            ->where('entries.0.entry_title', 'Falls back')
            ->where('entries.1.entry_title', 'Has published_at')));
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

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page->loadDeferredProps(fn (Assert $loaded) => $loaded
            ->where('entries', function ($entries) {
                $byTitle = collect($entries)->keyBy('entry_title');

                expect($byTitle['Custom row']['display_title'])->toBe('My nickname');
                expect($byTitle['Pds row']['display_title'])->toBe('PDS title');
                expect($byTitle['Feed-title row']['display_title'])->toBe('Feed title');
                expect($byTitle['Url row']['display_title'])->toBe('https://url.example/rss');

                return true;
            })));
});

test('serves the persisted favicon_url when discovery has populated it', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create([
        'feed_url' => 'https://blog.example.com/feed.xml',
        'favicon_url' => 'https://blog.example.com/static/icon.svg',
        'favicon_checked_at' => now(),
    ]);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);
    makeFeedEntry($feed, ['title' => 'Hi', 'published_at' => now()]);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page->loadDeferredProps(fn (Assert $loaded) => $loaded
            ->where('entries.0.favicon_url', 'https://blog.example.com/static/icon.svg')));
});

test('falls back to /favicon.ico when favicon_url has not been discovered yet', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/feed.xml']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);
    makeFeedEntry($feed, ['title' => 'Hi', 'published_at' => now()]);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page->loadDeferredProps(fn (Assert $loaded) => $loaded
            ->where('entries.0.favicon_url', 'https://blog.example.com/favicon.ico')));
});

test('returns null favicon_url when feed_url cannot be parsed and nothing is persisted', function () {
    $user = User::factory()->create();
    $feed = Feed::query()->create(['feed_url' => 'not-a-real-url']);
    Subscription::query()->create(['user_id' => $user->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);
    makeFeedEntry($feed, ['title' => 'Hi', 'published_at' => now()]);

    $this->actingAs(freshenBluesky($user));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page->loadDeferredProps(fn (Assert $loaded) => $loaded
            ->where('entries.0.favicon_url', null)));
});

test('does not render entries from other users\' subscriptions', function () {
    $me = User::factory()->create();
    $other = User::factory()->create();

    $feed = Feed::query()->create(['feed_url' => 'https://a.example/rss']);
    Subscription::query()->create(['user_id' => $other->did, 'feed_id' => $feed->id, 'at_uri' => 'at://x/a']);
    makeFeedEntry($feed, ['title' => 'Theirs', 'published_at' => now()]);

    $this->actingAs(freshenBluesky($me));

    $this->get(route('consume'))
        ->assertInertia(fn (Assert $page) => $page
            ->where('has_subscriptions', false)
            ->loadDeferredProps(fn (Assert $loaded) => $loaded->has('entries', 0)));
});
