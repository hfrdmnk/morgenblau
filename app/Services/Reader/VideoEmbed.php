<?php

namespace App\Services\Reader;

// Resolves a video URL into { embed_url, thumbnail_url }, or null when no provider matches.
class VideoEmbed
{
    private const YOUTUBE_HOSTS = ['youtube.com', 'www.youtube.com', 'm.youtube.com', 'youtu.be'];

    private const YOUTUBE_ID_PATTERN = '/^[A-Za-z0-9_-]{11}$/';

    /**
     * @return array{embed_url: string, thumbnail_url: string}|null
     */
    public static function resolve(?string $link): ?array
    {
        if ($link === null || $link === '') {
            return null;
        }

        $videoId = self::extractYouTubeId($link);
        if ($videoId === null) {
            return null;
        }

        return [
            'embed_url' => "https://www.youtube-nocookie.com/embed/{$videoId}?autoplay=1&rel=0",
            'thumbnail_url' => "https://i.ytimg.com/vi/{$videoId}/hqdefault.jpg",
        ];
    }

    private static function extractYouTubeId(string $link): ?string
    {
        $parts = parse_url($link);
        if (! is_array($parts) || ! isset($parts['host'])) {
            return null;
        }

        $host = strtolower($parts['host']);
        if (! in_array($host, self::YOUTUBE_HOSTS, true)) {
            return null;
        }

        $path = $parts['path'] ?? '/';

        if ($host === 'youtu.be') {
            $candidate = ltrim($path, '/');

            return self::validId($candidate);
        }

        if (str_starts_with($path, '/watch')) {
            parse_str($parts['query'] ?? '', $query);
            $candidate = is_string($query['v'] ?? null) ? $query['v'] : '';

            return self::validId($candidate);
        }

        if (preg_match('~^/(?:embed|v|shorts)/([^/?#]+)~', $path, $matches) === 1) {
            return self::validId($matches[1]);
        }

        return null;
    }

    private static function validId(string $candidate): ?string
    {
        return preg_match(self::YOUTUBE_ID_PATTERN, $candidate) === 1 ? $candidate : null;
    }
}
