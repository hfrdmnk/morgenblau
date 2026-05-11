<?php

namespace App\Services\Feeds;

use App\Models\Feed;
use App\Models\Subscription;
use App\Models\User;
use Carbon\CarbonImmutable;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\DB;

class FeedRefreshScheduler
{
    private const MANUAL_IN_FLIGHT_WINDOW_MINUTES = 5;

    private const WAIT_POLL_INTERVAL_MICROSECONDS = 750_000;

    public function __construct(private readonly FeedJobDispatcher $dispatcher) {}

    /**
     * Block until every subscribed feed dispatched at or after $since has
     * finished its fetch, or the timeout elapses. Used inside Inertia's
     * deferred-prop closure so /consume shows the skeleton until the digest
     * is current.
     */
    public function waitForPendingFetches(User $user, CarbonImmutable $since, int $timeoutSeconds = 18): void
    {
        $deadline = microtime(true) + $timeoutSeconds;

        $subscribedFeedIds = Subscription::query()
            ->where('user_id', $user->did)
            ->pluck('feed_id');

        if ($subscribedFeedIds->isEmpty()) {
            return;
        }

        while (microtime(true) < $deadline) {
            $pending = DB::table('feeds')
                ->whereIn('id', $subscribedFeedIds)
                ->where('last_dispatched_at', '>=', $since)
                ->where(function ($q) {
                    $q->whereNull('last_fetched_at')
                        ->orWhereColumn('last_fetched_at', '<', 'last_dispatched_at');
                })
                ->exists();

            if (! $pending) {
                return;
            }

            usleep(self::WAIT_POLL_INTERVAL_MICROSECONDS);
        }
    }

    public function dispatchAll(): int
    {
        return $this->dispatchFor(user: null, force: false);
    }

    public function dispatchForUser(User $user): int
    {
        return $this->dispatchFor(user: $user, force: true);
    }

    /**
     * Manual refresh ($force=true) bypasses next_check_at but keeps a short
     * in-flight guard so rapid clicks don't double-dispatch.
     */
    private function dispatchFor(?User $user, bool $force): int
    {
        $now = Date::now();

        $query = Feed::query()
            ->whereNull('disabled_at')
            ->whereHas('subscriptions', function ($q) use ($user) {
                if ($user !== null) {
                    $q->where('subscriptions.user_id', $user->did);
                }
            });

        if ($force) {
            $inFlightSince = $now->copy()->subMinutes(self::MANUAL_IN_FLIGHT_WINDOW_MINUTES);
            $query->where(fn ($q) => $q->whereNull('last_dispatched_at')
                ->orWhere('last_dispatched_at', '<', $inFlightSince));
        } else {
            $query->where(fn ($q) => $q->whereNull('next_check_at')
                ->orWhere('next_check_at', '<=', $now));
        }

        $dispatched = 0;
        foreach ($query->get() as $feed) {
            if ($this->dispatcher->dispatch($feed->id)) {
                $dispatched++;
            }
        }

        return $dispatched;
    }
}
