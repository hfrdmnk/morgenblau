<?php

namespace App\Console\Commands;

use App\Jobs\RefreshFeedJob;
use App\Models\Feed;
use Illuminate\Console\Command;

class RetryDisabledFeedsCommand extends Command
{
    protected $signature = 'feeds:retry-disabled';

    protected $description = 'Attempt a single fetch for each muted feed; silently re-enable on success.';

    public function handle(): int
    {
        $count = 0;

        Feed::query()
            ->whereNotNull('disabled_at')
            ->whereHas('subscriptions')
            ->get()
            ->each(function (Feed $feed) use (&$count) {
                RefreshFeedJob::dispatch($feed->id);
                $count++;
            });

        $this->info("Dispatched retry for {$count} muted feed(s).");

        return self::SUCCESS;
    }
}
