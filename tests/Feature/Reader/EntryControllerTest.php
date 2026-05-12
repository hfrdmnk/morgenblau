<?php

use App\Enums\ContentType;
use App\Jobs\ExtractArticleJob;
use App\Models\Feed;
use App\Models\FeedEntry;
use App\Models\User;
use App\Services\Feeds\EntrySlugger;
use Illuminate\Support\Facades\Queue;
use Inertia\Testing\AssertableInertia as Assert;

beforeEach(fn () => Queue::fake());

function makeReaderEntry(Feed $feed, array $overrides = []): FeedEntry
{
    return FeedEntry::query()->create(array_merge([
        'feed_id' => $feed->id,
        'guid' => 'urn:'.bin2hex(random_bytes(4)),
        'title' => 'A quiet morning',
        'link' => 'https://example.com/post',
        'summary' => 'Summary',
        'content' => '<p>The body of the post.</p>',
        'author' => 'Ada',
        'published_at' => now()->subHour(),
        'content_type' => ContentType::Blogpost->value,
        'first_seen_at' => now(),
        'updated_at' => now(),
    ], $overrides));
}

test('renders the reader page for a blogpost with a feed body', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create([
        'feed_url' => 'https://blog.example.com/rss',
        'title' => 'Example Blog',
    ]);
    $entry = makeReaderEntry($feed, [
        'title' => 'A quiet morning',
        'content' => '<p>The body of the post.</p>',
    ]);

    $this->actingAs($user);

    $this->get(route('entry.show', $entry->entry_slug))
        ->assertOk()
        ->assertInertia(fn (Assert $page) => $page
            ->component('entry')
            ->where('entry.entry_slug', $entry->entry_slug)
            ->where('entry.title', 'A quiet morning')
            ->where('entry.feed_body', '<p>The body of the post.</p>')
            ->where('entry.source_url', 'https://example.com/post')
            ->where('entry.source_domain', 'example.com')
            ->where('entry.feed.display_title', 'Example Blog'));
});

test('returns 404 for an unknown slug', function () {
    $user = freshenBluesky(User::factory()->create());

    $this->actingAs($user)
        ->get(route('entry.show', 'doesnotexist'))
        ->assertNotFound();
});

test('returns 404 when the slug resolves to a non-blogpost entry', function (string $contentType) {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeReaderEntry($feed, ['content_type' => $contentType]);

    $this->actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertNotFound();
})->with([
    'microblog' => ContentType::Microblog->value,
    'video' => ContentType::Video->value,
    'podcast' => ContentType::Podcast->value,
]);

test('guests are redirected to the login page', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeReaderEntry($feed);

    $this->get(route('entry.show', $entry->entry_slug))
        ->assertRedirect(route('login'));
});

test('EntrySlugger is deterministic for the same (feed_id, guid)', function () {
    expect(EntrySlugger::for(42, 'urn:abc'))
        ->toBe(EntrySlugger::for(42, 'urn:abc'))
        ->and(EntrySlugger::for(42, 'urn:abc'))
        ->not->toBe(EntrySlugger::for(43, 'urn:abc'))
        ->and(EntrySlugger::for(42, 'urn:abc'))
        ->toHaveLength(10);
});

test('substantial-content entry does not dispatch the extraction job', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeReaderEntry($feed, [
        'content' => '<p>'.str_repeat('Word ', 500).'</p>',
        'summary' => 'Distinct short summary',
    ]);

    $this->actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertOk()
        ->assertInertia(fn (Assert $page) => $page
            ->where('entry.auto_choice', 'feed')
            ->where('entry.extraction_state', 'not_attempted'));

    Queue::assertNothingPushed();
});

test('summary-only entry dispatches the extraction job exactly once', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeReaderEntry($feed, [
        'content' => '<p>Short.</p>',
        'summary' => 'Short.',
    ]);

    $this->actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertOk()
        ->assertInertia(fn (Assert $page) => $page
            ->where('entry.auto_choice', 'feed')
            ->where('entry.extraction_state', 'pending'));

    Queue::assertPushed(ExtractArticleJob::class, 1);
});

test('renders the extracted body when one is cached on the entry', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create([
        'feed_url' => 'https://blog.example.com/rss',
        'title' => 'Example Blog',
    ]);
    $entry = makeReaderEntry($feed, [
        'content' => '<p>Short.</p>',
        'summary' => 'Short.',
        'extracted_html' => '<p>Full extracted article body.</p>',
        'extracted_at' => now(),
        'extraction_attempts' => 1,
        'extraction_attempted_at' => now(),
    ]);

    $this->actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertOk()
        ->assertInertia(fn (Assert $page) => $page
            ->where('entry.auto_choice', 'extracted')
            ->where('entry.extraction_state', 'available')
            ->where('entry.extracted_body', '<p>Full extracted article body.</p>'));

    Queue::assertNothingPushed();
});

test('does not redispatch when the previous attempt is still within the backoff window', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeReaderEntry($feed, [
        'content' => '<p>Short.</p>',
        'summary' => 'Short.',
        'extraction_attempts' => 1,
        'extraction_attempted_at' => now()->subMinute(),
        'extraction_failure_reason' => 'unreachable',
    ]);

    $this->actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertOk()
        ->assertInertia(fn (Assert $page) => $page
            ->where('entry.extraction_state', 'failed'));

    Queue::assertNothingPushed();
});

test('does not dispatch when the entry has been permanently flagged', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeReaderEntry($feed, [
        'content' => '<p>Short.</p>',
        'summary' => 'Short.',
        'extraction_attempts' => 20,
        'extraction_attempted_at' => now()->subDays(7),
        'extraction_failure_reason' => 'unreachable',
    ]);

    $this->actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertOk();

    Queue::assertNothingPushed();
});
