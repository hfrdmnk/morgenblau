<?php

use App\Enums\ContentType;
use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\FeedEntry;
use App\Services\Feeds\ConditionalFeedClient;
use App\Services\Feeds\FeedEntryUpserter;
use App\Services\Feeds\FeedFetcher;
use App\Services\Feeds\Processors\ProcessorPipeline;
use FeedIo\FeedIo;
use Tests\Doubles\StubFeedClient;

test('end-to-end: Modified result flows fetch → classify → sanitize → upsert', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://example.com/encoded.rss']);

    $fixture = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/content-encoded.rss.xml');
    $stub = new StubFeedClient(['https://example.com/encoded.rss' => $fixture]);

    app()->instance(FeedIo::class, new FeedIo($stub));
    app()->instance(ConditionalFeedClient::class, $stub);

    (new RefreshFeedJob($feed->id))->handle(
        app(FeedFetcher::class),
        app(FeedEntryUpserter::class),
        app(ProcessorPipeline::class),
    );

    $row = FeedEntry::query()->where('feed_id', $feed->id)->firstOrFail();

    expect($row->content_type)->toBe(ContentType::Blogpost);
    expect($row->content)->toContain('<p>Real prose with a <strong>bold</strong> word.</p>');
    expect($row->content)->not->toContain('<script');
    expect($row->content)->not->toContain('alert(');
    expect($row->content)->not->toContain('<iframe');
    expect($row->content)->not->toContain('onclick');
});

test('end-to-end: YouTube channel feed entries land as Video', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://www.youtube.com/feeds/videos.xml?channel_id=UCxxxxx']);

    $fixture = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/youtube.atom.xml');
    $stub = new StubFeedClient(['https://www.youtube.com/feeds/videos.xml?channel_id=UCxxxxx' => $fixture]);

    app()->instance(FeedIo::class, new FeedIo($stub));
    app()->instance(ConditionalFeedClient::class, $stub);

    (new RefreshFeedJob($feed->id))->handle(
        app(FeedFetcher::class),
        app(FeedEntryUpserter::class),
        app(ProcessorPipeline::class),
    );

    $rows = FeedEntry::query()->where('feed_id', $feed->id)->get();

    expect($rows)->toHaveCount(2);
    expect($rows->every(fn ($r) => $r->content_type === ContentType::Video))->toBeTrue();
});

test('end-to-end: podcast feed entries land as Podcast', function () {
    $feed = Feed::query()->create(['feed_url' => 'https://example.com/podcast.rss']);

    $fixture = (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/podcast.rss.xml');
    $stub = new StubFeedClient(['https://example.com/podcast.rss' => $fixture]);

    app()->instance(FeedIo::class, new FeedIo($stub));
    app()->instance(ConditionalFeedClient::class, $stub);

    (new RefreshFeedJob($feed->id))->handle(
        app(FeedFetcher::class),
        app(FeedEntryUpserter::class),
        app(ProcessorPipeline::class),
    );

    $rows = FeedEntry::query()->where('feed_id', $feed->id)->get();

    expect($rows)->toHaveCount(2);
    expect($rows->every(fn ($r) => $r->content_type === ContentType::Podcast))->toBeTrue();
});
