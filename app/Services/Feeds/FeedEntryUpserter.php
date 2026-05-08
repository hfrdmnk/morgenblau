<?php

namespace App\Services\Feeds;

use App\Data\Feeds\ProcessedEntryData;
use App\Models\Feed;
use App\Models\FeedEntry;
use Illuminate\Support\Facades\Date;

class FeedEntryUpserter
{
    private const MUTABLE_COLUMNS = [
        'title',
        'link',
        'summary',
        'content',
        'author',
        'published_at',
        'content_type',
        'metadata',
        'updated_at',
    ];

    /**
     * @param  iterable<ProcessedEntryData>  $entries
     * @return array{inserted: int, updated: int}
     */
    public function upsert(Feed $feed, iterable $entries): array
    {
        $now = Date::now();
        $rowsByGuid = [];

        foreach ($entries as $entry) {
            $guid = $this->resolveGuid($entry);
            if ($guid === null) {
                continue;
            }

            $rowsByGuid[$guid] = [
                'feed_id' => $feed->id,
                'guid' => $guid,
                'title' => $entry->title,
                'link' => $entry->link,
                'summary' => $entry->summary,
                'content' => $entry->content,
                'author' => $entry->author,
                'published_at' => $entry->publishedAt,
                'content_type' => $entry->contentType->value,
                'metadata' => $entry->metadata === [] ? null : json_encode($entry->metadata),
                'first_seen_at' => $now,
                'updated_at' => $now,
            ];
        }

        if ($rowsByGuid === []) {
            return ['inserted' => 0, 'updated' => 0];
        }

        $rows = array_values($rowsByGuid);
        $guids = array_keys($rowsByGuid);

        $existingGuids = FeedEntry::query()
            ->where('feed_id', $feed->id)
            ->whereIn('guid', $guids)
            ->pluck('guid')
            ->all();

        $updated = count($existingGuids);
        $inserted = count($rows) - $updated;

        FeedEntry::query()->upsert($rows, ['feed_id', 'guid'], self::MUTABLE_COLUMNS);

        return ['inserted' => $inserted, 'updated' => $updated];
    }

    private function resolveGuid(ProcessedEntryData $entry): ?string
    {
        if ($entry->guid !== null && $entry->guid !== '') {
            return $entry->guid;
        }

        if ($entry->link !== null && $entry->link !== '') {
            return $entry->link;
        }

        return null;
    }
}
