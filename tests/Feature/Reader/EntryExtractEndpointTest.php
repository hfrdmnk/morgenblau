<?php

use App\Data\Reader\ExtractionResult;
use App\Enums\ContentType;
use App\Enums\Reader\ExtractionFailureReason;
use App\Models\Feed;
use App\Models\FeedEntry;
use App\Models\User;
use App\Services\Reader\ArticleExtractor;
use Inertia\Testing\AssertableInertia as Assert;
use Mockery\MockInterface;

use function Pest\Laravel\actingAs;

function makeBlogpostEntry(Feed $feed, array $overrides = []): FeedEntry
{
    return FeedEntry::query()->create(array_merge([
        'feed_id' => $feed->id,
        'guid' => 'urn:'.bin2hex(random_bytes(4)),
        'title' => 'A quiet morning',
        'link' => 'https://example.com/post',
        'summary' => 'Summary',
        'content' => '<p>Short.</p>',
        'author' => 'Ada',
        'published_at' => now()->subHour(),
        'content_type' => ContentType::Blogpost->value,
        'first_seen_at' => now(),
        'updated_at' => now(),
    ], $overrides));
}

function bindFakeExtractor(ExtractionResult $result): MockInterface
{
    $mock = Mockery::mock(ArticleExtractor::class);
    $mock->shouldReceive('extract')->andReturn($result);
    app()->instance(ArticleExtractor::class, $mock);

    return $mock;
}

test('manual extract persists html and redirects to entry.show with the available state', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeBlogpostEntry($feed);

    bindFakeExtractor(ExtractionResult::success(
        html: '<p>Full extracted body.</p>',
        title: 'A quiet morning',
        author: 'Ada',
        imageUrl: null,
        wordCount: 320,
        readingTimeSeconds: 90,
    ));

    actingAs($user)
        ->post(route('entry.extract', $entry->entry_slug))
        ->assertRedirect(route('entry.show', $entry->entry_slug));

    $entry->refresh();
    expect($entry->extracted_html)->toBe('<p>Full extracted body.</p>');
    expect($entry->extracted_at)->not->toBeNull();
    expect($entry->extraction_attempts)->toBe(1);
    expect($entry->extraction_failure_reason)->toBeNull();

    actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertInertia(fn (Assert $page) => $page
            ->component('entry')
            ->where('entry.extraction_state', 'available')
            ->where('entry.auto_choice', 'extracted')
            ->where('entry.extracted_body', '<p>Full extracted body.</p>'));
});

test('manual extract records the failure reason and exposes the failed state', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeBlogpostEntry($feed);

    bindFakeExtractor(ExtractionResult::failure(ExtractionFailureReason::Unreachable));

    actingAs($user)
        ->post(route('entry.extract', $entry->entry_slug))
        ->assertRedirect(route('entry.show', $entry->entry_slug));

    $entry->refresh();
    expect($entry->extracted_html)->toBeNull();
    expect($entry->extraction_attempts)->toBe(1);
    expect($entry->extraction_failure_reason)->toBe('unreachable');

    actingAs($user)
        ->get(route('entry.show', $entry->entry_slug))
        ->assertInertia(fn (Assert $page) => $page
            ->where('entry.extraction_state', 'failed')
            ->where('entry.extracted_body', null));
});

test('manual extract bypasses the permanent-failure threshold', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeBlogpostEntry($feed, [
        'extraction_attempts' => 20,
        'extraction_attempted_at' => now()->subDays(7),
        'extraction_failure_reason' => 'unreachable',
    ]);

    $mock = Mockery::mock(ArticleExtractor::class);
    $mock->shouldReceive('extract')
        ->once()
        ->andReturn(ExtractionResult::success(
            html: '<p>Recovered body.</p>',
            title: null,
            author: null,
            imageUrl: null,
            wordCount: 50,
            readingTimeSeconds: 14,
        ));
    app()->instance(ArticleExtractor::class, $mock);

    actingAs($user)
        ->post(route('entry.extract', $entry->entry_slug))
        ->assertRedirect(route('entry.show', $entry->entry_slug));

    $entry->refresh();
    expect($entry->extracted_html)->toBe('<p>Recovered body.</p>');
    expect($entry->extraction_attempts)->toBe(21);
});

test('manual extract bypasses the backoff window when a previous attempt is fresh', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeBlogpostEntry($feed, [
        'extraction_attempts' => 1,
        'extraction_attempted_at' => now()->subSeconds(30),
        'extraction_failure_reason' => 'unreachable',
    ]);

    $mock = Mockery::mock(ArticleExtractor::class);
    $mock->shouldReceive('extract')
        ->once()
        ->andReturn(ExtractionResult::success(
            html: '<p>Body.</p>',
            title: null,
            author: null,
            imageUrl: null,
            wordCount: 5,
            readingTimeSeconds: 1,
        ));
    app()->instance(ArticleExtractor::class, $mock);

    actingAs($user)
        ->post(route('entry.extract', $entry->entry_slug))
        ->assertRedirect(route('entry.show', $entry->entry_slug));
});

test('manual extract fires even when the auto-decide rule would not', function () {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);

    // Substantial content + distinct summary — auto rule returns false.
    $entry = makeBlogpostEntry($feed, [
        'content' => '<p>'.str_repeat('Word ', 500).'</p>',
        'summary' => 'Distinct short summary',
    ]);

    $mock = Mockery::mock(ArticleExtractor::class);
    $mock->shouldReceive('extract')
        ->once()
        ->andReturn(ExtractionResult::success(
            html: '<p>Clean extract.</p>',
            title: null,
            author: null,
            imageUrl: null,
            wordCount: 5,
            readingTimeSeconds: 1,
        ));
    app()->instance(ArticleExtractor::class, $mock);

    actingAs($user)
        ->post(route('entry.extract', $entry->entry_slug))
        ->assertRedirect(route('entry.show', $entry->entry_slug));

    $entry->refresh();
    expect($entry->extracted_html)->toBe('<p>Clean extract.</p>');
});

test('manual extract returns 404 for an unknown slug', function () {
    $user = freshenBluesky(User::factory()->create());

    actingAs($user)
        ->post(route('entry.extract', 'doesnotexist'))
        ->assertNotFound();
});

test('manual extract returns 404 for a non-blogpost entry', function (string $contentType) {
    $user = freshenBluesky(User::factory()->create());
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeBlogpostEntry($feed, ['content_type' => $contentType]);

    actingAs($user)
        ->post(route('entry.extract', $entry->entry_slug))
        ->assertNotFound();
})->with([
    'microblog' => ContentType::Microblog->value,
    'video' => ContentType::Video->value,
    'podcast' => ContentType::Podcast->value,
]);

test('guests are redirected to login when calling the manual extract endpoint', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://blog.example.com/rss']);
    $entry = makeBlogpostEntry($feed);

    $this->post(route('entry.extract', $entry->entry_slug))
        ->assertRedirect(route('login'));
});
