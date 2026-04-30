<?php

namespace App\Services;

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

    /**
     * @return non-empty-list<ResolvedFeed>
     */
    public function discover(string $url): array
    {
        return $this->feedResolver->resolve($url);
    }

    public function create(User $user, OAuthSession $session, ChosenFeed $choice): SubscriptionResult
    {
        $response = Bluesky::withToken($session)
            ->client(auth: true)
            ->createRecord(
                repo: $user->did,
                collection: self::COLLECTION,
                record: $this->buildRecord($choice),
            );

        if ($response->failed()) {
            throw new RuntimeException("PDS write failed: {$response->status()} {$response->body()}");
        }

        return new SubscriptionResult(
            title: $choice->title ?? $choice->feedUrl,
            atUri: $response->json('uri'),
        );
    }

    /**
     * @return array<string, mixed>
     */
    private function buildRecord(ChosenFeed $choice): array
    {
        return array_filter([
            'feedUrl' => $choice->feedUrl,
            'title' => $choice->title,
            'siteUrl' => $choice->siteUrl,
            'sourceType' => $choice->sourceType,
            'createdAt' => Date::now()->toIso8601String(),
        ], fn ($value): bool => $value !== null);
    }
}
