<?php

namespace App\Repositories;

use App\Data\Digest\DigestStatusData;
use App\Models\FeedEntry;
use App\Models\Subscription;
use App\Models\User;
use Carbon\CarbonImmutable;
use Illuminate\Support\Facades\DB;

class DigestStatusRepository
{
    /**
     * Snapshot of the user's digest fetch state relative to a moment in time.
     *
     * Pending: any feed they subscribe to was dispatched after `since` and
     * has not since reported back as fetched. Filters cron-only dispatches
     * that happened before the user's action.
     */
    public function forUser(User $user, CarbonImmutable $since): DigestStatusData
    {
        $subscribedFeedIds = Subscription::query()
            ->where('user_id', $user->did)
            ->pluck('feed_id');

        if ($subscribedFeedIds->isEmpty()) {
            return new DigestStatusData(pending: false, newCount: 0, latestEntryAt: null);
        }

        $pending = DB::table('feeds')
            ->whereIn('id', $subscribedFeedIds)
            ->where('last_dispatched_at', '>', $since)
            ->where(function ($q) {
                $q->whereNull('last_fetched_at')
                    ->orWhereColumn('last_fetched_at', '<', 'last_dispatched_at');
            })
            ->exists();

        $newRow = FeedEntry::query()
            ->whereIn('feed_id', $subscribedFeedIds)
            ->where('first_seen_at', '>', $since)
            ->selectRaw('COUNT(*) as new_count, MAX(first_seen_at) as latest_entry_at')
            ->first();

        $newCount = (int) ($newRow->new_count ?? 0);
        $latestEntryAt = $newRow->latest_entry_at !== null
            ? CarbonImmutable::parse($newRow->latest_entry_at)->toIso8601String()
            : null;

        return new DigestStatusData(
            pending: $pending,
            newCount: $newCount,
            latestEntryAt: $latestEntryAt,
        );
    }
}
