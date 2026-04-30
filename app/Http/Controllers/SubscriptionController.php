<?php

namespace App\Http\Controllers;

use App\Http\Requests\StoreSubscriptionRequest;
use App\Services\FeedAdapters\Exceptions\UnresolvableFeedException;
use App\Services\SubscriptionService;
use Illuminate\Http\RedirectResponse;
use Illuminate\Validation\ValidationException;
use Revolution\Bluesky\Session\OAuthSession;

class SubscriptionController extends Controller
{
    public function __construct(private readonly SubscriptionService $subscriptions) {}

    public function store(StoreSubscriptionRequest $request): RedirectResponse
    {
        $data = $request->validated();
        $isPrivate = (bool) ($data['is_private'] ?? false);

        $session = OAuthSession::create(
            $request->session()->get('bluesky_session', []),
        );

        try {
            $result = $this->subscriptions->add(
                user: $request->user(),
                session: $session,
                url: $data['url'],
                isPrivate: $isPrivate,
            );
        } catch (UnresolvableFeedException $e) {
            throw ValidationException::withMessages(['url' => $e->getMessage()]);
        }

        return back()->with('flash', [
            'message' => "Subscribed to {$result->title}.",
        ]);
    }
}
