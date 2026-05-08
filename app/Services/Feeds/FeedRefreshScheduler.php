<?php

namespace App\Services\Feeds;

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use App\Models\User;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\Log;
use Throwable;

class FeedRefreshScheduler
{
    private const MANUAL_IN_FLIGHT_WINDOW_MINUTES = 5;

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

        $feedIds = $query->pluck('feeds.id')->all();

        $dispatched = 0;
        foreach ($feedIds as $feedId) {
            try {
                RefreshFeedJob::dispatch((int) $feedId);
                Feed::query()->where('id', $feedId)->update(['last_dispatched_at' => $now]);
                $dispatched++;
            } catch (Throwable $e) {
                Log::warning('RefreshFeedJob dispatch failed', [
                    'feed_id' => $feedId,
                    'error' => $e->getMessage(),
                ]);
            }
        }

        return $dispatched;
    }
}
