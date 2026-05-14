<?php

use App\Services\Reader\VideoEmbed;

test('returns null for null and empty input', function (?string $link) {
    expect(VideoEmbed::resolve($link))->toBeNull();
})->with([
    'null' => null,
    'empty' => '',
]);

test('returns null for an unsupported provider', function () {
    expect(VideoEmbed::resolve('https://vimeo.com/12345678'))->toBeNull();
});

test('resolves a canonical YouTube watch URL', function () {
    $resolved = VideoEmbed::resolve('https://www.youtube.com/watch?v=dQw4w9WgXcQ');

    expect($resolved)->toBe([
        'embed_url' => 'https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ?autoplay=1&rel=0',
        'thumbnail_url' => 'https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg',
    ]);
});

test('preserves the video id across YouTube URL variants', function (string $link) {
    expect(VideoEmbed::resolve($link)['embed_url'] ?? '')->toContain('dQw4w9WgXcQ');
})->with([
    'watch' => 'https://www.youtube.com/watch?v=dQw4w9WgXcQ',
    'watch+t' => 'https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42',
    'mobile' => 'https://m.youtube.com/watch?v=dQw4w9WgXcQ',
    'youtu.be' => 'https://youtu.be/dQw4w9WgXcQ',
    'embed' => 'https://www.youtube.com/embed/dQw4w9WgXcQ',
    'shorts' => 'https://www.youtube.com/shorts/dQw4w9WgXcQ',
]);

test('returns null for malformed YouTube ids', function (string $link) {
    expect(VideoEmbed::resolve($link))->toBeNull();
})->with([
    'too short' => 'https://www.youtube.com/watch?v=abc',
    'too long' => 'https://www.youtube.com/watch?v=dQw4w9WgXcQabcdef',
    'no v param' => 'https://www.youtube.com/watch?foo=bar',
]);
