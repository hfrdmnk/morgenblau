<?php

namespace App\Services\Feeds;

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\User;
use Illuminate\Support\Facades\Date;

class FeedRefreshScheduler
{
    private const IN_FLIGHT_WINDOW_MINUTES = 5;

    /**
     * Dispatch RefreshFeedJob for every feed with at least one local subscription.
     * Returns the number of feeds dispatched.
     *
     * Pre-marks last_dispatched_at = now() for the exact id set so the in-flight
     * signal is accurate even before workers pick up the jobs. Dispatches
     * unconditionally; ShouldBeUnique on the job drops in-flight duplicates at the
     * queue layer.
     */
    public function dispatchAll(): int
    {
        $feedIds = Feed::query()
            ->join('subscriptions', 'subscriptions.feed_id', '=', 'feeds.id')
            ->distinct()
            ->pluck('feeds.id')
            ->all();

        if ($feedIds === []) {
            return 0;
        }

        Feed::query()->whereIn('id', $feedIds)->update(['last_dispatched_at' => Date::now()]);

        foreach ($feedIds as $feedId) {
            RefreshFeedJob::dispatch((int) $feedId);
        }

        return count($feedIds);
    }

    /**
     * Dispatch RefreshFeedJob for the given user's feeds that aren't already in
     * flight (last_dispatched_at within the last 5 minutes). Returns the number
     * of feeds dispatched.
     */
    public function dispatchForUser(User $user): int
    {
        $now = Date::now();
        $inFlightSince = $now->copy()->subMinutes(self::IN_FLIGHT_WINDOW_MINUTES);

        $feedIds = Feed::query()
            ->join('subscriptions', 'subscriptions.feed_id', '=', 'feeds.id')
            ->where('subscriptions.user_id', $user->did)
            ->where(fn ($q) => $q->whereNull('feeds.last_dispatched_at')
                ->orWhere('feeds.last_dispatched_at', '<', $inFlightSince))
            ->distinct()
            ->pluck('feeds.id')
            ->all();

        if ($feedIds === []) {
            return 0;
        }

        Feed::query()->whereIn('id', $feedIds)->update(['last_dispatched_at' => $now]);

        foreach ($feedIds as $feedId) {
            RefreshFeedJob::dispatch((int) $feedId);
        }

        return count($feedIds);
    }
}
