<?php

namespace App\Http\Controllers;

use App\Data\Feeds\FeedEntryViewData;
use App\Enums\ContentType;
use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\FeedEntry;
use App\Services\Subscriptions\SubscriptionService;
use Carbon\CarbonImmutable;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\DB;
use Inertia\Inertia;
use Inertia\Response;
use Spatie\LaravelData\DataCollection;

class ConsumeController extends Controller
{
    private const ENTRY_LIMIT = 200;

    private const STALE_AFTER_MINUTES = 60;

    private const IN_FLIGHT_WINDOW_MINUTES = 5;

    public function __construct(private readonly SubscriptionService $subscriptions) {}

    public function __invoke(Request $request): Response
    {
        $user = $request->user();

        // Force a PDS-backed reconcile of the local mirror before we read it.
        // Cached after the first call; cache miss runs reconcileLocalMirror.
        $hasSubscriptions = $this->subscriptions->listSubscriptions($user)->count() > 0;

        $this->dispatchStaleRefreshes($user->did);

        $entries = fn () => $this->loadEntries($user->did);
        $refreshingFeedIds = fn () => $this->loadRefreshingFeedIds($user->did);

        return Inertia::render('consume', [
            'entries' => fn () => FeedEntryViewData::collect($entries(), DataCollection::class),
            'refreshing_feed_ids' => $refreshingFeedIds,
            'has_subscriptions' => fn () => $hasSubscriptions,
        ]);
    }

    /**
     * @return list<FeedEntryViewData>
     */
    private function loadEntries(string $userDid): array
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
                'subscriptions.custom_title',
                'subscriptions.pds_title',
            ])
            ->join('feeds', 'feeds.id', '=', 'feed_entries.feed_id')
            ->join('subscriptions', 'subscriptions.feed_id', '=', 'feed_entries.feed_id')
            ->where('subscriptions.user_id', $userDid)
            ->orderByDesc(DB::raw('COALESCE(feed_entries.published_at, feed_entries.first_seen_at)'))
            ->limit(self::ENTRY_LIMIT)
            ->get();

        return $rows->map(fn ($row) => new FeedEntryViewData(
            id: (int) $row->id,
            feedId: (int) $row->feed_id,
            displayTitle: $row->custom_title ?: ($row->pds_title ?: ($row->feed_title ?: $row->feed_url)),
            entryTitle: $row->entry_title,
            link: $row->link,
            summary: $row->summary,
            author: $row->author,
            publishedAt: $row->published_at !== null ? CarbonImmutable::parse($row->published_at) : null,
            firstSeenAt: CarbonImmutable::parse($row->first_seen_at),
            contentType: $row->content_type instanceof ContentType ? $row->content_type : ContentType::from((string) $row->content_type),
        ))->all();
    }

    private function dispatchStaleRefreshes(string $userDid): void
    {
        $now = Date::now();
        $staleBefore = $now->copy()->subMinutes(self::STALE_AFTER_MINUTES);
        $inFlightSince = $now->copy()->subMinutes(self::IN_FLIGHT_WINDOW_MINUTES);

        $staleFeedIds = Feed::query()
            ->join('subscriptions', 'subscriptions.feed_id', '=', 'feeds.id')
            ->where('subscriptions.user_id', $userDid)
            ->where(fn ($q) => $q->whereNull('feeds.last_fetched_at')
                ->orWhere('feeds.last_fetched_at', '<', $staleBefore))
            ->where(fn ($q) => $q->whereNull('feeds.last_dispatched_at')
                ->orWhere('feeds.last_dispatched_at', '<', $inFlightSince))
            ->distinct()
            ->pluck('feeds.id')
            ->all();

        if ($staleFeedIds === []) {
            return;
        }

        Feed::query()->whereIn('id', $staleFeedIds)->update(['last_dispatched_at' => $now]);

        foreach ($staleFeedIds as $feedId) {
            RefreshFeedJob::dispatch((int) $feedId);
        }
    }

    /**
     * @return list<int>
     */
    private function loadRefreshingFeedIds(string $userDid): array
    {
        $inFlightSince = Date::now()->subMinutes(self::IN_FLIGHT_WINDOW_MINUTES);

        $ids = Feed::query()
            ->join('subscriptions', 'subscriptions.feed_id', '=', 'feeds.id')
            ->where('subscriptions.user_id', $userDid)
            ->whereNotNull('feeds.last_dispatched_at')
            ->where('feeds.last_dispatched_at', '>', $inFlightSince)
            ->distinct()
            ->pluck('feeds.id')
            ->all();

        return array_values(array_map('intval', $ids));
    }
}
