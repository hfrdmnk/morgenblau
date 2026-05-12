<?php

namespace App\Services\Reader;

use App\Models\FeedEntry;

/**
 * Pure decision: should the reader auto-fetch the full article from the source
 * URL because the RSS feed only shipped a summary?
 *
 * Threshold of 1500 chars is tunable (not a SPEC commitment); 1499 → true,
 * 1500 → false. Strip tags first so HTML markup doesn't game the count.
 */
final class AutoExtractDecider
{
    private const SUBSTANTIAL_CONTENT_LENGTH = 1500;

    public static function shouldAutoExtract(FeedEntry $entry): bool
    {
        $content = $entry->content;

        if ($content === null || $content === '') {
            return true;
        }

        $stripped = strip_tags($content);

        if (mb_strlen($stripped) < self::SUBSTANTIAL_CONTENT_LENGTH) {
            return true;
        }

        $summary = $entry->summary;
        if ($summary !== null && $summary !== '' && $stripped === strip_tags($summary)) {
            return true;
        }

        return false;
    }
}
