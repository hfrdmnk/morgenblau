<?php

namespace App\Services;

use App\Models\Subscription;
use App\Models\User;
use App\Services\FeedAdapters\FeedResolver;
use App\Services\FeedAdapters\ResolvedFeed;
use Illuminate\Support\Facades\Date;
use Revolution\Bluesky\Facades\Bluesky;
use Revolution\Bluesky\Session\OAuthSession;
use RuntimeException;

class SubscriptionService
{
    private const COLLECTION = 'app.skyreader.feed.subscription';

    public function __construct(private readonly FeedResolver $feedResolver) {}

    public function add(User $user, OAuthSession $session, string $url, bool $isPrivate): SubscriptionResult
    {
        $resolved = $this->feedResolver->resolve($url);

        if ($isPrivate) {
            $subscription = Subscription::create([
                'user_did' => $user->did,
                'feed_url' => $resolved->feedUrl,
                'title' => $resolved->title,
                'site_url' => $resolved->siteUrl,
                'category' => $resolved->category,
                'source_type' => $resolved->sourceType,
                'is_private' => true,
            ]);

            return new SubscriptionResult(
                title: $resolved->title ?? $resolved->feedUrl,
                isPrivate: true,
                atUri: null,
                subscription: $subscription,
            );
        }

        $response = Bluesky::withToken($session)
            ->client(auth: true)
            ->createRecord(
                repo: $user->did,
                collection: self::COLLECTION,
                record: $this->buildRecord($resolved),
            );

        if ($response->failed()) {
            throw new RuntimeException("PDS write failed: {$response->status()} {$response->body()}");
        }

        return new SubscriptionResult(
            title: $resolved->title ?? $resolved->feedUrl,
            isPrivate: false,
            atUri: $response->json('uri'),
            subscription: null,
        );
    }

    /**
     * @return array<string, mixed>
     */
    private function buildRecord(ResolvedFeed $resolved): array
    {
        return array_filter([
            'feedUrl' => $resolved->feedUrl,
            'title' => $resolved->title,
            'siteUrl' => $resolved->siteUrl,
            'category' => $resolved->category,
            'sourceType' => $resolved->sourceType,
            'createdAt' => Date::now()->toIso8601String(),
        ], fn ($value): bool => $value !== null);
    }
}
