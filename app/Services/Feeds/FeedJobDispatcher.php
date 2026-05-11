<?php

namespace App\Services\Feeds;

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Log;
use Throwable;

class FeedJobDispatcher
{
    /**
     * Stamp last_dispatched_at and queue the refresh atomically. The stamp
     * must land before dispatch — on sync queues the job runs immediately
     * and nulls the column via markFetched/etc., so a later stamp would
     * leave the feed permanently flagged as in-flight. Wrapping both in a
     * transaction means a dispatch failure rolls the stamp back so the
     * next manual refresh can pick the feed up instead of waiting out the
     * in-flight window with no job actually queued.
     */
    public function dispatch(int $feedId): bool
    {
        try {
            DB::transaction(function () use ($feedId): void {
                Feed::query()->whereKey($feedId)->update(['last_dispatched_at' => Date::now()]);
                RefreshFeedJob::dispatch($feedId);
            });

            return true;
        } catch (Throwable $e) {
            Log::warning('refresh feed job dispatch failed', [
                'feed_id' => $feedId,
                'error' => $e->getMessage(),
            ]);

            return false;
        }
    }
}
