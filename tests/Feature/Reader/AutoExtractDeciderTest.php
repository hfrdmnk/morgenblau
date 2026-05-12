<?php

use App\Models\Feed;
use App\Models\FeedEntry;
use App\Services\Reader\AutoExtractDecider;

function makeEntryForDecider(array $overrides = []): FeedEntry
{
    $feed = Feed::query()->create([
        'feed_url' => 'https://blog.example.com/rss',
    ]);

    return FeedEntry::query()->create(array_merge([
        'feed_id' => $feed->id,
        'guid' => 'urn:'.bin2hex(random_bytes(4)),
        'title' => 'Sample',
        'link' => 'https://example.com/post',
        'summary' => 'short summary',
        'content' => str_repeat('Substantial article content. ', 200),
        'content_type' => 'blogpost',
        'first_seen_at' => now(),
        'updated_at' => now(),
    ], $overrides));
}

test('substantial unique content returns false', function () {
    $entry = makeEntryForDecider([
        'content' => '<p>'.str_repeat('Word ', 1000).'</p>',
        'summary' => 'different short summary',
    ]);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeFalse();
});

test('short content returns true', function () {
    $entry = makeEntryForDecider([
        'content' => '<p>'.str_repeat('Word ', 30).'</p>',
        'summary' => 'something else',
    ]);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeTrue();
});

test('content identical to summary returns true regardless of length', function () {
    $body = str_repeat('Word ', 1000);
    $entry = makeEntryForDecider([
        'content' => $body,
        'summary' => $body,
    ]);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeTrue();
});

test('empty content returns true', function () {
    $entry = makeEntryForDecider(['content' => '']);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeTrue();
});

test('null content returns true', function () {
    $entry = makeEntryForDecider(['content' => null]);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeTrue();
});

test('1499 chars of stripped content returns true (below threshold)', function () {
    $content = str_repeat('a', 1499);
    $entry = makeEntryForDecider([
        'content' => $content,
        'summary' => 'distinct summary',
    ]);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeTrue();
});

test('1500 chars of stripped content returns false (at threshold)', function () {
    $content = str_repeat('a', 1500);
    $entry = makeEntryForDecider([
        'content' => $content,
        'summary' => 'distinct summary',
    ]);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeFalse();
});

test('HTML tags do not count toward the threshold', function () {
    // 1499 visible chars wrapped in <p> tags should still be below threshold.
    $entry = makeEntryForDecider([
        'content' => '<p>'.str_repeat('a', 1499).'</p>',
        'summary' => 'distinct',
    ]);

    expect(AutoExtractDecider::shouldAutoExtract($entry))->toBeTrue();
});
