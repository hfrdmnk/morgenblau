<?php

namespace App\Http\Controllers\Feeds;

use App\Http\Controllers\Controller;
use App\Services\Feeds\FeedRefreshScheduler;
use App\Services\Subscriptions\SubscriptionService;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;
use Throwable;

class FeedRefreshController extends Controller
{
    public function __construct(
        private readonly FeedRefreshScheduler $scheduler,
        private readonly SubscriptionService $subscriptions,
    ) {}

    public function __invoke(Request $request): RedirectResponse
    {
        $user = $request->user();

        try {
            $this->subscriptions->reconcile($user);
        } catch (Throwable $e) {
            Log::warning('subscriptions reconcile on manual refresh failed', [
                'did' => $user->did,
                'error' => $e->getMessage(),
            ]);
        }

        $this->scheduler->dispatchForUser($user);

        return back();
    }
}
