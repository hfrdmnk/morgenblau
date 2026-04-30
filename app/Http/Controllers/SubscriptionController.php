<?php

namespace App\Http\Controllers;

use App\Http\Requests\DiscoverSubscriptionRequest;
use App\Http\Requests\StoreSubscriptionRequest;
use App\Services\ChosenFeed;
use App\Services\FeedAdapters\Exceptions\UnresolvableFeedException;
use App\Services\FeedAdapters\ResolvedFeed;
use App\Services\SubscriptionService;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\RedirectResponse;
use Illuminate\Validation\ValidationException;
use Revolution\Bluesky\Session\OAuthSession;

class SubscriptionController extends Controller
{
    public function __construct(private readonly SubscriptionService $subscriptions) {}

    public function discover(DiscoverSubscriptionRequest $request): JsonResponse
    {
        try {
            $candidates = $this->subscriptions->discover($request->validated()['url']);
        } catch (UnresolvableFeedException $e) {
            throw ValidationException::withMessages(['url' => $e->getMessage()]);
        }

        return response()->json([
            'candidates' => array_map(fn (ResolvedFeed $c) => [
                'feed_url' => $c->feedUrl,
                'title' => $c->title,
                'site_url' => $c->siteUrl,
                'source_type' => $c->sourceType,
            ], $candidates),
        ]);
    }

    public function store(StoreSubscriptionRequest $request): RedirectResponse
    {
        $data = $request->validated();

        $session = OAuthSession::create(
            $request->session()->get('bluesky_session', []),
        );

        $result = $this->subscriptions->create(
            user: $request->user(),
            session: $session,
            choice: new ChosenFeed(
                feedUrl: $data['feed_url'],
                title: $data['title'] ?? null,
                siteUrl: $data['site_url'] ?? null,
                sourceType: $data['source_type'],
            ),
        );

        return back()->with('flash', [
            'message' => "Subscribed to {$result->title}.",
        ]);
    }
}
