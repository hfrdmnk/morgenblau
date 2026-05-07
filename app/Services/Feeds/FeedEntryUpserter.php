<?php

namespace App\Services\Feeds;

use App\Data\Feeds\FetchedEntryData;
use App\Models\Feed;
use App\Models\FeedEntry;
use Illuminate\Support\Facades\Date;

class FeedEntryUpserter
{
    /**
     * @param  iterable<FetchedEntryData>  $entries
     * @return array{inserted: int, updated: int}
     */
    public function upsert(Feed $feed, iterable $entries): array
    {
        $now = Date::now();
        $inserted = 0;
        $updated = 0;

        foreach ($entries as $entry) {
            $guid = $entry->guid !== null && $entry->guid !== ''
                ? $entry->guid
                : ($entry->link !== null && $entry->link !== '' ? $entry->link : null);

            if ($guid === null) {
                continue;
            }

            $existing = FeedEntry::query()
                ->where('feed_id', $feed->id)
                ->where('guid', $guid)
                ->first();

            if ($existing === null) {
                FeedEntry::query()->create([
                    'feed_id' => $feed->id,
                    'guid' => $guid,
                    'title' => $entry->title,
                    'link' => $entry->link,
                    'summary' => $entry->summary,
                    'content' => $entry->content,
                    'author' => $entry->author,
                    'published_at' => $entry->publishedAt,
                    'first_seen_at' => $now,
                    'updated_at' => $now,
                ]);
                $inserted++;

                continue;
            }

            $existing->fill([
                'title' => $entry->title,
                'link' => $entry->link,
                'summary' => $entry->summary,
                'content' => $entry->content,
                'author' => $entry->author,
                'published_at' => $entry->publishedAt,
                'updated_at' => $now,
            ])->save();
            $updated++;
        }

        return ['inserted' => $inserted, 'updated' => $updated];
    }
}
