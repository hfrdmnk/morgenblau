<?php

use App\Data\Feeds\FeedEnclosureData;
use App\Data\Feeds\FetchedEntryData;
use App\Data\Feeds\ProcessedEntryData;
use App\Enums\ContentType;
use App\Models\Feed;
use App\Services\Feeds\Processors\ContentTypeClassifier;

function classify(Feed $feed, FetchedEntryData $entry): ContentType
{
    $classifier = app(ContentTypeClassifier::class);
    $processed = ProcessedEntryData::fromFetched($entry);

    return $classifier->process($processed, $feed)->contentType;
}

function makeEntry(array $overrides = []): FetchedEntryData
{
    $defaults = [
        'title' => 'A title',
        'link' => 'https://example.com/post',
        'guid' => 'guid',
        'summary' => null,
        'content' => 'Content body',
        'author' => null,
        'enclosures' => null,
    ];
    $resolved = array_replace($defaults, array_intersect_key($overrides, $defaults));

    return new FetchedEntryData(
        title: $resolved['title'],
        link: $resolved['link'],
        guid: $resolved['guid'],
        summary: $resolved['summary'],
        content: $resolved['content'],
        author: $resolved['author'],
        publishedAt: null,
        enclosures: $resolved['enclosures'],
    );
}

dataset('classifier_cases', [
    'youtube channel feed → video' => [
        fn () => Feed::query()->create(['feed_url' => 'https://www.youtube.com/feeds/videos.xml?channel_id=UCx']),
        fn () => makeEntry(['title' => 'Some video', 'content' => 'Description']),
        ContentType::Video,
    ],
    'audio enclosure → podcast' => [
        fn () => Feed::query()->create(['feed_url' => 'https://example.com/podcast.rss']),
        fn () => makeEntry([
            'enclosures' => [new FeedEnclosureData(url: 'https://example.com/ep.mp3', type: 'audio/mpeg', length: 10)],
        ]),
        ContentType::Podcast,
    ],
    'empty title + short content → microblog' => [
        fn () => Feed::query()->create(['feed_url' => 'https://example.com/micro.rss']),
        fn () => makeEntry(['title' => '', 'content' => 'Just a quick thought.']),
        ContentType::Microblog,
    ],
    'null title + short content → microblog' => [
        fn () => Feed::query()->create(['feed_url' => 'https://example.com/micro.rss']),
        fn () => makeEntry(['title' => null, 'content' => 'Tiny.']),
        ContentType::Microblog,
    ],
    'title + content → blogpost' => [
        fn () => Feed::query()->create(['feed_url' => 'https://example.com/blog.rss']),
        fn () => makeEntry(['title' => 'Long form post', 'content' => str_repeat('Lorem ipsum dolor sit amet. ', 30)]),
        ContentType::Blogpost,
    ],
    'empty title but long content → blogpost' => [
        fn () => Feed::query()->create(['feed_url' => 'https://example.com/blog.rss']),
        fn () => makeEntry(['title' => '', 'content' => str_repeat('A', 300)]),
        ContentType::Blogpost,
    ],
]);

test('classifies according to heuristics', function (Closure $feed, Closure $entry, ContentType $expected) {
    expect(classify($feed(), $entry()))->toBe($expected);
})->with('classifier_cases');
