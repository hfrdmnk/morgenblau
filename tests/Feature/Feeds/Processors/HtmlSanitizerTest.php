<?php

use App\Data\Feeds\FetchedEntryData;
use App\Data\Feeds\ProcessedEntryData;
use App\Enums\ContentType;
use App\Models\Feed;
use App\Services\Feeds\Processors\HtmlSanitizer;

function sanitize(string $html, ContentType $type): string
{
    $sanitizer = app(HtmlSanitizer::class);
    $feed = Feed::query()->create(['feed_url' => 'https://example.com/rss']);

    $entry = new FetchedEntryData(
        title: 't',
        link: 'https://example.com/p',
        guid: 'g',
        summary: null,
        content: $html,
        author: null,
        publishedAt: null,
        enclosures: null,
    );

    $processed = ProcessedEntryData::fromFetched($entry)->withContentType($type);

    return (string) $sanitizer->process($processed, $feed)->content;
}

dataset('sanitizer_cases', [
    'script tag removed (blogpost)' => [
        '<p>Hi</p><script>alert(1)</script>',
        ContentType::Blogpost,
        fn (string $out) => expect($out)->toContain('<p>Hi</p>')->and($out)->not->toContain('<script')->and($out)->not->toContain('alert(1)'),
    ],
    'iframe removed (blogpost)' => [
        '<p>Hi</p><iframe src="https://evil.example.com"></iframe>',
        ContentType::Blogpost,
        fn (string $out) => expect($out)->toContain('<p>Hi</p>')->and($out)->not->toContain('<iframe'),
    ],
    'onclick attribute stripped (blogpost)' => [
        '<p onclick="alert(1)">Hi</p>',
        ContentType::Blogpost,
        fn (string $out) => expect($out)->toContain('Hi')->and($out)->not->toContain('onclick'),
    ],
    'allow-listed link preserved (blogpost)' => [
        '<p><a href="https://example.com">link</a></p>',
        ContentType::Blogpost,
        fn (string $out) => expect($out)->toContain('<a href="https://example.com">link</a>'),
    ],
    'img preserved on blogpost' => [
        '<p><img src="https://example.com/x.png" alt="x"></p>',
        ContentType::Blogpost,
        fn (string $out) => expect($out)->toContain('<img'),
    ],
    'img stripped on microblog' => [
        '<p><img src="https://example.com/x.png" alt="x"></p>',
        ContentType::Microblog,
        fn (string $out) => expect($out)->not->toContain('<img'),
    ],
    'h2 preserved on blogpost' => [
        '<h2>Heading</h2><p>Body</p>',
        ContentType::Blogpost,
        fn (string $out) => expect($out)->toContain('<h2>Heading</h2>'),
    ],
    'h2 stripped on microblog' => [
        '<h2>Heading</h2><p>Body</p>',
        ContentType::Microblog,
        fn (string $out) => expect($out)->not->toContain('<h2')->and($out)->toContain('Heading'),
    ],
]);

test('sanitizes HTML according to content-type preset', function (string $html, ContentType $type, Closure $assertion) {
    $assertion(sanitize($html, $type));
})->with('sanitizer_cases');
