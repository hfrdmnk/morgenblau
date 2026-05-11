<?php

namespace App\Http\Controllers;

use App\Models\Subscription;
use App\Queries\Digest\EntriesQuery;
use App\Services\Feeds\FeedRefreshScheduler;
use Carbon\CarbonImmutable;
use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Response;

class ConsumeController extends Controller
{
    public function __invoke(
        Request $request,
        EntriesQuery $entries,
        FeedRefreshScheduler $scheduler,
    ): Response {
        $user = $request->user();
        $hasSubscriptions = Subscription::query()->where('user_id', $user->did)->exists();

        return Inertia::render('consume', [
            // Pull happens inside the closure because deferred props execute on
            // a follow-up partial-reload request — pulling out here would clear
            // the session value before the deferred run ever sees it.
            'entries' => Inertia::defer(function () use ($entries, $scheduler, $user, $request) {
                $pending = $request->session()->pull('fetch_action_at');

                if (is_string($pending)) {
                    $scheduler->waitForPendingFetches($user, CarbonImmutable::parse($pending));
                }

                return $entries->forUser($user);
            }),
            'has_subscriptions' => $hasSubscriptions,
        ]);
    }
}
