<?php

namespace App\Services\Feeds;

/**
 * Pure, deterministic slug generator for FeedEntry rows.
 *
 * Same `(feed_id, guid)` pair always yields the same ~10-char base62 slug,
 * so backfills and re-runs are idempotent and inserts can compute the slug
 * without a round-trip to the database.
 */
final class EntrySlugger
{
    private const ALPHABET = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';

    private const LENGTH = 10;

    public static function for(int $feedId, string $guid): string
    {
        $digest = hash('sha256', $feedId.'|'.$guid, true);

        $out = '';
        for ($i = 0; $i < self::LENGTH; $i++) {
            $out .= self::ALPHABET[ord($digest[$i]) % 62];
        }

        return $out;
    }
}
