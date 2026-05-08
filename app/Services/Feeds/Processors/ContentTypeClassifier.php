<?php

namespace App\Services\Feeds\Processors;

use App\Data\Feeds\ProcessedEntryData;
use App\Enums\ContentType;
use App\Models\Feed;

class ContentTypeClassifier implements EntryProcessor
{
    private const YOUTUBE_FEED_PATTERN = '~^https?://(www\.)?youtube\.com/feeds/videos\.xml~i';

    private const MICROBLOG_LENGTH_THRESHOLD = 280;

    public function process(ProcessedEntryData $entry, Feed $feed): ProcessedEntryData
    {
        return $entry->withContentType($this->classify($entry, $feed));
    }

    private function classify(ProcessedEntryData $entry, Feed $feed): ContentType
    {
        if (preg_match(self::YOUTUBE_FEED_PATTERN, (string) $feed->feed_url) === 1) {
            return ContentType::Video;
        }

        foreach ($entry->enclosures as $enclosure) {
            if ($enclosure->type !== null && str_starts_with(strtolower($enclosure->type), 'audio/')) {
                return ContentType::Podcast;
            }
        }

        $title = trim((string) $entry->title);
        $textBody = strip_tags((string) ($entry->content ?? $entry->summary ?? ''));

        if ($title === '' && mb_strlen($textBody) < self::MICROBLOG_LENGTH_THRESHOLD) {
            return ContentType::Microblog;
        }

        return ContentType::Blogpost;
    }
}
