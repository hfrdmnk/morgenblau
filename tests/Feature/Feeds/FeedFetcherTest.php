<?php

use App\Data\Feeds\FetchedEntryData;
use App\Exceptions\FeedFetchException;
use App\Services\Feeds\FeedFetcher;
use Carbon\CarbonImmutable;
use FeedIo\FeedIo;
use Tests\Doubles\StubFeedClient;

function feedFetcherWith(array $bodies): FeedFetcher
{
    return new FeedFetcher(new FeedIo(new StubFeedClient($bodies)));
}

function sampleRssFixture(): string
{
    return (string) file_get_contents(__DIR__.'/../../Fixtures/feeds/sample.rss.xml');
}

test('it parses an RSS feed into FetchedEntryData', function () {
    $fetcher = feedFetcherWith(['https://example.com/feed.xml' => sampleRssFixture()]);

    $entries = $fetcher->fetch('https://example.com/feed.xml');

    expect($entries)->toHaveCount(2)
        ->and($entries[0])->toBeInstanceOf(FetchedEntryData::class);

    $first = $entries[0];
    expect($first->title)->toBe('First post')
        ->and($first->link)->toBe('https://example.com/posts/first')
        ->and($first->guid)->toBe('post-1')
        ->and($first->content)->toBe('Short summary of the first post.')
        ->and($first->summary)->toBeNull()
        ->and($first->author)->toBe('Jane Doe')
        ->and($first->publishedAt)->toBeInstanceOf(CarbonImmutable::class)
        ->and($first->publishedAt->toIso8601String())->toBe('2026-04-15T09:30:00+00:00');
});

test('it tolerates items missing optional fields', function () {
    $fetcher = feedFetcherWith(['https://example.com/feed.xml' => sampleRssFixture()]);

    $second = $fetcher->fetch('https://example.com/feed.xml')[1];

    expect($second->title)->toBe('Bare item')
        ->and($second->link)->toBe('https://example.com/posts/bare')
        // feed-io falls back to the link when an item has no <guid>
        ->and($second->guid)->toBe('https://example.com/posts/bare')
        ->and($second->summary)->toBeNull()
        ->and($second->content)->toBeNull()
        ->and($second->author)->toBeNull()
        ->and($second->publishedAt)->toBeNull();
});

test('it wraps feed-io errors in FeedFetchException', function () {
    $fetcher = feedFetcherWith([]);

    expect(fn () => $fetcher->fetch('https://example.com/missing.xml'))
        ->toThrow(FeedFetchException::class);
});
