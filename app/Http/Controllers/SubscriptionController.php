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
use Illuminate\Http\Request;
use Illuminate\Validation\ValidationException;
use Revolution\Bluesky\Session\OAuthSession;
use RuntimeException;

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

        $existingFeedUrls = $this->subscriptions->listFeedUrls(
            $request->user(),
            $this->oauthSession($request),
        );

        return response()->json([
            'candidates' => array_map(fn (ResolvedFeed $c) => [
                'feed_url' => $c->feedUrl,
                'title' => $c->title,
                'site_url' => $c->siteUrl,
                'source_type' => $c->sourceType,
            ], $candidates),
            'existing_feed_urls' => $existingFeedUrls,
        ]);
    }

    public function store(StoreSubscriptionRequest $request): RedirectResponse
    {
        $items = $request->validated()['subscriptions'];
        $session = $this->oauthSession($request);
        $user = $request->user();

        $existing = array_flip($this->subscriptions->listFeedUrls($user, $session));

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
                    choice: new ChosenFeed(
                        feedUrl: $item['feed_url'],
                        title: $item['title'] ?? null,
                        siteUrl: $item['site_url'] ?? null,
                        sourceType: $item['source_type'],
                    ),
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
