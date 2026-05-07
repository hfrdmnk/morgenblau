<?php

namespace App\Http\Controllers\Feeds;

use App\Http\Controllers\Controller;
use App\Services\Feeds\FeedRefreshScheduler;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;

class FeedRefreshController extends Controller
{
    public function __construct(private readonly FeedRefreshScheduler $scheduler) {}

    public function __invoke(Request $request): RedirectResponse
    {
        $this->scheduler->dispatchForUser($request->user());

        return back();
    }
}
