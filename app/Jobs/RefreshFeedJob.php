<?php

namespace App\Jobs;

use App\Models\Feed;
use App\Services\Feeds\FeedEntryUpserter;
use App\Services\Feeds\FeedFetcher;
use Illuminate\Contracts\Queue\ShouldBeUnique;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Queue\Queueable;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Str;
use Throwable;

class RefreshFeedJob implements ShouldBeUnique, ShouldQueue
{
    use Queueable;

    public int $tries = 1;

    public int $uniqueFor = 300;

    public function __construct(public readonly int $feedId) {}

    public function uniqueId(): string
    {
        return (string) $this->feedId;
    }

    public function handle(FeedFetcher $fetcher, FeedEntryUpserter $upserter): void
    {
        $feed = Feed::query()->find($this->feedId);

        if ($feed === null) {
            return;
        }

        try {
            $entries = $fetcher->fetch($feed->feed_url);
            $upserter->upsert($feed, $entries);
        } catch (Throwable $e) {
            $feed->forceFill([
                'last_failed_at' => Date::now(),
                'last_error' => Str::limit($e->getMessage(), 500, ''),
                'last_dispatched_at' => null,
            ])->save();

            throw $e;
        }

        $feed->forceFill([
            'last_fetched_at' => Date::now(),
            'last_dispatched_at' => null,
            'last_failed_at' => null,
            'last_error' => null,
        ])->save();
    }
}
