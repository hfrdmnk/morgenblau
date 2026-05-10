<?php

namespace App\Repositories;

use App\Data\Feeds\FeedEntryViewData;
use App\Enums\ContentType;
use App\Models\FeedEntry;
use App\Models\Subscription;
use App\Models\User;
use Carbon\CarbonImmutable;
use Illuminate\Support\Facades\DB;
use Spatie\LaravelData\DataCollection;

class FeedEntryRepository
{
    /**
     * @return DataCollection<int, FeedEntryViewData>
     */
    public function digestEntries(User $user, int $limit = 200): DataCollection
    {
        $rows = FeedEntry::query()
            ->select([
                'feed_entries.id',
                'feed_entries.feed_id',
                'feed_entries.title as entry_title',
                'feed_entries.link',
                'feed_entries.summary',
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

        $items = $rows->map(fn ($row) => new FeedEntryViewData(
            id: (int) $row->id,
            feedId: (int) $row->feed_id,
            displayTitle: Subscription::resolveDisplayTitle(
                $row->custom_title,
                $row->pds_title,
                $row->feed_title,
                $row->feed_url,
            ),
            entryTitle: $row->entry_title,
            link: $row->link,
            summary: $row->summary,
            author: $row->author,
            publishedAt: $row->published_at !== null ? CarbonImmutable::parse($row->published_at) : null,
            firstSeenAt: CarbonImmutable::parse($row->first_seen_at),
            contentType: $row->content_type instanceof ContentType
                ? $row->content_type
                : ContentType::from((string) $row->content_type),
            faviconUrl: $row->favicon_url ?? self::deriveFaviconUrl((string) $row->feed_url),
        ))->all();

        return FeedEntryViewData::collect($items, DataCollection::class);
    }

    /**
     * Fallback when feeds.favicon_url is still null (newly subscribed feed
     * before its first refresh, or discovery legitimately failed). The
     * frontend's onError handler degrades to a globe icon if /favicon.ico
     * doesn't resolve.
     */
    private static function deriveFaviconUrl(string $feedUrl): ?string
    {
        $parts = parse_url($feedUrl);
        if ($parts === false || ! isset($parts['scheme'], $parts['host'])) {
            return null;
        }

        return $parts['scheme'].'://'.$parts['host'].'/favicon.ico';
    }
}
