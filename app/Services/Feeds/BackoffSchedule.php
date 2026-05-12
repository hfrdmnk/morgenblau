<?php

namespace App\Services\Feeds;

/**
 * Shared backoff curve for any subsystem that retries against an upstream
 * publisher (feed fetches, readability extraction). SPEC <feed-sources>:
 * 5min → 15min → 1h → 6h → 24h cap, ~20 attempts before permanent fail.
 */
final class BackoffSchedule
{
    public const MUTE_THRESHOLD = 20;

    public static function stepMinutes(int $failures): int
    {
        return match (true) {
            $failures <= 1 => 5,
            $failures === 2 => 15,
            $failures === 3 => 60,
            $failures === 4 => 360,
            default => 1440,
        };
    }

    public static function isPermanentlyFailed(int $failures): bool
    {
        return $failures >= self::MUTE_THRESHOLD;
    }
}
