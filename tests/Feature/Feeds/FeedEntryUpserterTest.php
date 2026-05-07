<?php

use App\Data\Feeds\FetchedEntryData;
use App\Models\Feed;
use App\Models\FeedEntry;
use App\Services\Feeds\FeedEntryUpserter;
use Carbon\CarbonImmutable;
use Illuminate\Support\Facades\Date;

beforeEach(function () {
    $this->feed = Feed::query()->create(['feed_url' => 'https://example.com/rss']);
    $this->upserter = app(FeedEntryUpserter::class);
});

test('inserts new entries', function () {
    $entries = [
        new FetchedEntryData(
            title: 'Morning post',
            link: 'https://example.com/morning',
            guid: 'urn:1',
            summary: 'A summary',
            content: 'Body',
            author: 'Alice',
            publishedAt: CarbonImmutable::parse('2026-05-07 06:00:00'),
        ),
        new FetchedEntryData(
            title: 'Evening post',
            link: 'https://example.com/evening',
            guid: 'urn:2',
            summary: null,
            content: null,
            author: null,
            publishedAt: null,
        ),
    ];

    $result = $this->upserter->upsert($this->feed, $entries);

    expect($result)->toBe(['inserted' => 2, 'updated' => 0]);
    expect(FeedEntry::query()->count())->toBe(2);
});

test('dedupes on guid across upsert calls', function () {
    $entry = new FetchedEntryData(
        title: 'Post',
        link: 'https://example.com/post',
        guid: 'urn:1',
        summary: null,
        content: null,
        author: null,
        publishedAt: null,
    );

    $this->upserter->upsert($this->feed, [$entry]);
    $second = $this->upserter->upsert($this->feed, [$entry]);

    expect($second)->toBe(['inserted' => 0, 'updated' => 1]);
    expect(FeedEntry::query()->count())->toBe(1);
});

test('skips entries with neither guid nor link', function () {
    $entries = [
        new FetchedEntryData(
            title: 'Orphan',
            link: null,
            guid: null,
            summary: null,
            content: null,
            author: null,
            publishedAt: null,
        ),
        new FetchedEntryData(
            title: 'Empty strings',
            link: '',
            guid: '',
            summary: null,
            content: null,
            author: null,
            publishedAt: null,
        ),
    ];

    $result = $this->upserter->upsert($this->feed, $entries);

    expect($result)->toBe(['inserted' => 0, 'updated' => 0]);
    expect(FeedEntry::query()->count())->toBe(0);
});

test('falls back to link when guid is null', function () {
    $entry = new FetchedEntryData(
        title: 'Post',
        link: 'https://example.com/post',
        guid: null,
        summary: null,
        content: null,
        author: null,
        publishedAt: null,
    );

    $this->upserter->upsert($this->feed, [$entry]);

    expect(FeedEntry::query()->where('guid', 'https://example.com/post')->exists())->toBeTrue();
});

test('upsert preserves first_seen_at while updating mutable fields', function () {
    $original = new FetchedEntryData(
        title: 'Old title',
        link: 'https://example.com/post',
        guid: 'urn:1',
        summary: 'Old summary',
        content: 'Old content',
        author: 'Old author',
        publishedAt: CarbonImmutable::parse('2026-05-01 00:00:00'),
    );

    Date::setTestNow('2026-05-01 12:00:00');
    $this->upserter->upsert($this->feed, [$original]);

    $row = FeedEntry::query()->where('guid', 'urn:1')->firstOrFail();
    $firstSeen = $row->first_seen_at;

    Date::setTestNow('2026-05-07 12:00:00');
    $updated = new FetchedEntryData(
        title: 'New title',
        link: 'https://example.com/post-renamed',
        guid: 'urn:1',
        summary: 'New summary',
        content: 'New content',
        author: 'New author',
        publishedAt: CarbonImmutable::parse('2026-05-07 06:00:00'),
    );
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
