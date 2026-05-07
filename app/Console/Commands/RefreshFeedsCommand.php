<?php

namespace App\Console\Commands;

use App\Services\Feeds\FeedRefreshScheduler;
use Illuminate\Console\Command;

class RefreshFeedsCommand extends Command
{
    protected $signature = 'feeds:refresh-all';

    protected $description = 'Dispatch RefreshFeedJob for every feed with at least one local subscription.';

    public function handle(FeedRefreshScheduler $scheduler): int
    {
        $count = $scheduler->dispatchAll();

        $this->info("Dispatched {$count} feed refresh job(s).");

        return self::SUCCESS;
    }
}
