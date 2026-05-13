<?php

namespace App\Services\Feeds\Processors;

use App\Data\Feeds\ProcessedEntryData;
use App\Enums\ContentType;
use App\Models\Feed;

/** ContentEncoded writes the body to `content`; copy to `summary` for microblogs whose renderer only reads summary. */
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
