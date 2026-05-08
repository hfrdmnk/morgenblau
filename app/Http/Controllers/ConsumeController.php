<?php

namespace App\Http\Controllers;

use App\Models\Subscription;
use App\Repositories\FeedEntryRepository;
use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Response;

class ConsumeController extends Controller
{
    public function __invoke(Request $request, FeedEntryRepository $entries): Response
    {
        $user = $request->user();
        $hasSubscriptions = Subscription::query()->where('user_id', $user->did)->exists();

        return Inertia::render('consume', [
            'entries' => fn () => $entries->digestEntries($user),
            'has_subscriptions' => $hasSubscriptions,
        ]);
    }
}
