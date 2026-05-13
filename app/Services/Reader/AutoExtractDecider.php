<?php

namespace App\Services\Reader;

use App\Models\FeedEntry;

/** Auto-fetch the source when the feed only shipped a stub. 300-char threshold is tunable. */
final class AutoExtractDecider
{
    private const SUBSTANTIAL_CONTENT_LENGTH = 300;

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
