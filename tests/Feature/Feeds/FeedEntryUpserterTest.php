<?php

use App\Data\Feeds\FetchedEntryData;
use App\Data\Feeds\ProcessedEntryData;
use App\Enums\ContentType;
use App\Models\Feed;
use App\Models\FeedEntry;
use App\Services\Feeds\FeedEntryUpserter;
use Carbon\CarbonImmutable;
use Illuminate\Support\Facades\Date;

function processed(array $overrides = []): ProcessedEntryData
{
    $defaults = [
        'title' => 'Title',
        'link' => 'https://example.com/post',
        'guid' => 'urn:1',
        'summary' => null,
        'content' => null,
        'author' => null,
        'publishedAt' => null,
    ];
    $resolved = array_replace($defaults, array_intersect_key($overrides, $defaults));

    $base = new FetchedEntryData(
        title: $resolved['title'],
        link: $resolved['link'],
        guid: $resolved['guid'],
        summary: $resolved['summary'],
        content: $resolved['content'],
        author: $resolved['author'],
        publishedAt: $resolved['publishedAt'],
        enclosures: null,
    );

    $p = ProcessedEntryData::fromFetched($base);

    if (isset($overrides['contentType'])) {
        $p = $p->withContentType($overrides['contentType']);
    }

    if (array_key_exists('metadata', $overrides)) {
        $p = new ProcessedEntryData(
            title: $p->title,
            link: $p->link,
            guid: $p->guid,
            summary: $p->summary,
            content: $p->content,
            author: $p->author,
            publishedAt: $p->publishedAt,
            contentType: $p->contentType,
            enclosures: $p->enclosures,
            metadata: $overrides['metadata'],
        );
    }

    return $p;
}

beforeEach(function () {
    $this->feed = Feed::query()->create(['feed_url' => 'https://example.com/rss']);
    $this->upserter = app(FeedEntryUpserter::class);
});

test('inserts new entries', function () {
    $entries = [
        processed([
            'title' => 'Morning post',
            'link' => 'https://example.com/morning',
            'guid' => 'urn:1',
            'summary' => 'A summary',
            'content' => 'Body',
            'author' => 'Alice',
            'publishedAt' => CarbonImmutable::parse('2026-05-07 06:00:00'),
        ]),
        processed([
            'title' => 'Evening post',
            'link' => 'https://example.com/evening',
            'guid' => 'urn:2',
        ]),
    ];

    $result = $this->upserter->upsert($this->feed, $entries);

    expect($result)->toBe(['inserted' => 2, 'updated' => 0]);
    expect(FeedEntry::query()->count())->toBe(2);
});

test('dedupes on guid across upsert calls', function () {
    $entry = processed(['guid' => 'urn:1', 'link' => 'https://example.com/post']);

    $this->upserter->upsert($this->feed, [$entry]);
    $second = $this->upserter->upsert($this->feed, [$entry]);

    expect($second)->toBe(['inserted' => 0, 'updated' => 1]);
    expect(FeedEntry::query()->count())->toBe(1);
});

test('skips entries with neither guid nor link', function () {
    $entries = [
        processed(['title' => 'Orphan', 'link' => null, 'guid' => null]),
        processed(['title' => 'Empty strings', 'link' => '', 'guid' => '']),
    ];

    $result = $this->upserter->upsert($this->feed, $entries);

    expect($result)->toBe(['inserted' => 0, 'updated' => 0]);
    expect(FeedEntry::query()->count())->toBe(0);
});

test('falls back to link when guid is null', function () {
    $entry = processed(['guid' => null, 'link' => 'https://example.com/post']);

    $this->upserter->upsert($this->feed, [$entry]);

    expect(FeedEntry::query()->where('guid', 'https://example.com/post')->exists())->toBeTrue();
});

test('upsert preserves first_seen_at while updating mutable fields', function () {
    $original = processed([
        'title' => 'Old title',
        'link' => 'https://example.com/post',
        'guid' => 'urn:1',
        'summary' => 'Old summary',
        'content' => 'Old content',
        'author' => 'Old author',
        'publishedAt' => CarbonImmutable::parse('2026-05-01 00:00:00'),
    ]);

    Date::setTestNow('2026-05-01 12:00:00');
    $this->upserter->upsert($this->feed, [$original]);

    $row = FeedEntry::query()->where('guid', 'urn:1')->firstOrFail();
    $firstSeen = $row->first_seen_at;

    Date::setTestNow('2026-05-07 12:00:00');
    $updated = processed([
        'title' => 'New title',
        'link' => 'https://example.com/post-renamed',
        'guid' => 'urn:1',
        'summary' => 'New summary',
        'content' => 'New content',
        'author' => 'New author',
        'publishedAt' => CarbonImmutable::parse('2026-05-07 06:00:00'),
    ]);
    $this->upserter->upsert($this->feed, [$updated]);

    $row->refresh();

    expect($row->title)->toBe('New title');
    expect($row->link)->toBe('https://example.com/post-renamed');
    expect($row->summary)->toBe('New summary');
    expect($row->content)->toBe('New content');
    expect($row->author)->toBe('New author');
    expect($row->published_at?->toDateTimeString())->toBe('2026-05-07 06:00:00');
    expect($row->first_seen_at?->toDateTimeString())->toBe($firstSeen->toDateTimeString());
    expect($row->updated_at?->toDateTimeString())->toBe('2026-05-07 12:00:00');
});

test('same guid in two different feeds produces two distinct rows', function () {
    $other = Feed::query()->create(['feed_url' => 'https://other.example.com/rss']);

    $entry = processed(['guid' => 'shared-guid', 'link' => 'https://example.com/p']);

    $this->upserter->upsert($this->feed, [$entry]);
    $this->upserter->upsert($other, [$entry]);

    expect(FeedEntry::query()->where('guid', 'shared-guid')->count())->toBe(2);
    expect(FeedEntry::query()->where('feed_id', $this->feed->id)->where('guid', 'shared-guid')->exists())->toBeTrue();
    expect(FeedEntry::query()->where('feed_id', $other->id)->where('guid', 'shared-guid')->exists())->toBeTrue();
});

test('content_type and metadata round-trip through upsert', function () {
    $entry = processed([
        'guid' => 'urn:1',
        'contentType' => ContentType::Podcast,
        'metadata' => ['duration' => 1234, 'enclosure_url' => 'https://example.com/audio.mp3'],
    ]);

    $this->upserter->upsert($this->feed, [$entry]);

    $row = FeedEntry::query()->where('guid', 'urn:1')->firstOrFail();

    expect($row->content_type)->toBe(ContentType::Podcast);
    expect($row->metadata)->toBe(['duration' => 1234, 'enclosure_url' => 'https://example.com/audio.mp3']);
});

test('dedupes within a single batch when duplicate guids appear', function () {
    $entries = [
        processed(['guid' => 'urn:1', 'title' => 'First']),
        processed(['guid' => 'urn:1', 'title' => 'Last write wins']),
    ];

    $result = $this->upserter->upsert($this->feed, $entries);

    expect($result)->toBe(['inserted' => 1, 'updated' => 0]);
    expect(FeedEntry::query()->where('guid', 'urn:1')->value('title'))->toBe('Last write wins');
});
