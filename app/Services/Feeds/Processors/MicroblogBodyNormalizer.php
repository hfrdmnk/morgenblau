<?php

namespace App\Services\Feeds\Processors;

use App\Data\Feeds\ProcessedEntryData;
use App\Enums\ContentType;
use App\Models\Feed;

/**
 * Mastodon-style feeds put the post body in <content:encoded>, not
 * <description> — so microblog entries land with content set but summary
 * null. The renderer only shows summary for microblogs, so copy content
 * across when summary is empty.
 */
class MicroblogBodyNormalizer implements EntryProcessor
{
    public function process(ProcessedEntryData $entry, Feed $feed): ProcessedEntryData
    {
        if ($entry->contentType !== ContentType::Microblog) {
            return $entry;
        }

        if ($entry->summary !== null && $entry->summary !== '') {
            return $entry;
        }

        if ($entry->content === null || $entry->content === '') {
            return $entry;
        }

        return $entry->withSanitizedContent($entry->content, $entry->content);
    }
}
