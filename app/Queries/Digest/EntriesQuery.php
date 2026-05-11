<?php

namespace App\Queries\Digest;

use App\Data\Feeds\FeedEntryViewData;
use App\Models\FeedEntry;
use App\Models\User;
use Illuminate\Support\Facades\DB;
use Spatie\LaravelData\DataCollection;

class EntriesQuery
{
    /**
     * @return DataCollection<int, FeedEntryViewData>
     */
    public function forUser(User $user, int $limit = 200): DataCollection
    {
        $rows = FeedEntry::query()
            ->select([
                'feed_entries.id',
                'feed_entries.feed_id',
                'feed_entries.title as entry_title',
                'feed_entries.link',
                'feed_entries.summary',
                'feed_entries.content',
                'feed_entries.author',
                'feed_entries.published_at',
                'feed_entries.first_seen_at',
                'feed_entries.content_type',
                'feeds.feed_url',
                'feeds.title as feed_title',
                'feeds.favicon_url',
                'subscriptions.custom_title',
                'subscriptions.pds_title',
            ])
            ->join('feeds', 'feeds.id', '=', 'feed_entries.feed_id')
            ->join('subscriptions', 'subscriptions.feed_id', '=', 'feed_entries.feed_id')
            ->where('subscriptions.user_id', $user->did)
            ->orderByDesc(DB::raw('COALESCE(feed_entries.published_at, feed_entries.first_seen_at)'))
            ->limit($limit)
            ->get();

        $items = $rows->map(fn ($row) => FeedEntryViewData::fromRow($row))->all();

        return FeedEntryViewData::collect($items, DataCollection::class);
    }
}
