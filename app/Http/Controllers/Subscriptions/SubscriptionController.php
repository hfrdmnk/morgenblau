<?php

namespace App\Http\Controllers\Subscriptions;

use App\Data\Feeds\ChosenFeedData;
use App\Http\Controllers\Controller;
use App\Http\Requests\Subscriptions\DiscoverRequest;
use App\Http\Requests\Subscriptions\StoreRequest;
use App\Services\Feeds\Exceptions\UnresolvableFeedException;
use App\Services\Subscriptions\SubscriptionService;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Validation\ValidationException;
use Revolution\Bluesky\Session\OAuthSession;
use RuntimeException;

class SubscriptionController extends Controller
{
    public function __construct(private readonly SubscriptionService $subscriptions) {}

    public function discover(DiscoverRequest $request): JsonResponse
    {
        try {
            $candidates = $this->subscriptions->discover($request->validated()['url']);
        } catch (UnresolvableFeedException $e) {
            throw ValidationException::withMessages(['url' => $e->getMessage()]);
        }

        $existingSubscriptions = $this->subscriptions->listSubscriptions(
            $request->user(),
            $this->oauthSession($request),
        );

        return response()->json([
            'candidates' => $candidates,
            'existing_subscriptions' => $existingSubscriptions,
        ]);
    }

    public function store(StoreRequest $request): RedirectResponse
    {
        $items = $request->validated()['subscriptions'];
        $session = $this->oauthSession($request);
        $user = $request->user();

        $existing = array_flip(
            $this->subscriptions->listSubscriptions($user, $session)
                ->toCollection()
                ->pluck('feedUrl')
                ->all(),
        );

        $duplicateErrors = [];
        foreach ($items as $index => $item) {
            if (isset($existing[$item['feed_url']])) {
                $duplicateErrors["subscriptions.{$index}.feed_url"] = 'You are already subscribed to this feed.';
            }
        }

        if ($duplicateErrors !== []) {
            throw ValidationException::withMessages($duplicateErrors);
        }

        $succeeded = [];
        $failed = [];

        foreach ($items as $item) {
            try {
                $result = $this->subscriptions->create(
                    user: $user,
                    session: $session,
                    choice: ChosenFeedData::from($item),
                );
                $succeeded[] = $result->title;
            } catch (RuntimeException) {
                $failed[] = $item['title'] ?: $item['feed_url'];
            }
        }

        return back()->with('flash', [
            'message' => $this->buildFlashMessage($succeeded, $failed),
        ]);
    }

    private function oauthSession(Request $request): OAuthSession
    {
        return OAuthSession::create(
            $request->session()->get('bluesky_session', []),
        );
    }

    /**
     * @param  array<int, string>  $succeeded
     * @param  array<int, string>  $failed
     */
    private function buildFlashMessage(array $succeeded, array $failed): string
    {
        $successCount = count($succeeded);
        $parts = [];

        if ($successCount === 1) {
            $parts[] = "Subscribed to {$succeeded[0]}.";
        } elseif ($successCount > 1) {
            $parts[] = "Subscribed to {$successCount} sources.";
        }

        if ($failed !== []) {
            $parts[] = 'Failed: '.implode(', ', $failed).'.';
        }

        return implode(' ', $parts);
    }
}
