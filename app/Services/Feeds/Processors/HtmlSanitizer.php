<?php

namespace App\Services\Feeds\Processors;

use App\Data\Feeds\ProcessedEntryData;
use App\Enums\ContentType;
use App\Models\Feed;
use Stevebauman\Purify\Facades\Purify;

class HtmlSanitizer implements EntryProcessor
{
    public function process(ProcessedEntryData $entry, Feed $feed): ProcessedEntryData
    {
        $preset = $this->presetFor($entry->contentType);

        $content = $entry->content !== null && $entry->content !== ''
            ? Purify::config($preset)->clean($entry->content)
            : $entry->content;

        $summary = $entry->summary !== null && $entry->summary !== ''
            ? Purify::config($preset)->clean($entry->summary)
            : $entry->summary;

        return $entry->withSanitizedContent($content, $summary);
    }

    private function presetFor(ContentType $type): string
    {
        return match ($type) {
            ContentType::Microblog => 'microblog',
            ContentType::Blogpost => 'blogpost',
            ContentType::Video => 'video',
            ContentType::Podcast => 'podcast',
        };
    }
}
