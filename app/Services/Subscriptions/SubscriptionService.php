<?php

namespace App\Services\Subscriptions;

use App\Data\Feeds\ChosenFeedData;
use App\Data\Feeds\ResolvedFeedData;
use App\Data\Subscriptions\ExistingSubscriptionData;
use App\Data\Subscriptions\SubscriptionResultData;
use App\Models\User;
use App\Services\Feeds\FeedResolver;
use Illuminate\Support\Facades\Date;
use RuntimeException;
use Spatie\LaravelData\DataCollection;

class SubscriptionService
{
    private const COLLECTION = 'app.skyreader.feed.subscription';

    public function __construct(private readonly FeedResolver $feedResolver) {}

    /**
     * @return non-empty-list<ResolvedFeedData>
     */
    public function discover(string $url): array
    {
        return $this->feedResolver->resolve($url);
    }

    /**
     * @return DataCollection<int, ExistingSubscriptionData>
     */
    public function listSubscriptions(User $user): DataCollection
    {
        $client = $user->bluesky()->client(auth: true);

        $subscriptions = [];
        $cursor = null;

        do {
            $response = $client->listRecords(
                repo: $user->did,
                collection: self::COLLECTION,
                limit: 100,
                cursor: $cursor,
            );

            if ($response->failed()) {
                throw new RuntimeException("PDS read failed: {$response->status()} {$response->body()}");
            }

            foreach ((array) $response->json('records', []) as $record) {
                $feedUrl = data_get($record, 'value.feedUrl');

                if (! is_string($feedUrl) || $feedUrl === '') {
                    continue;
                }

                $title = data_get($record, 'value.title');
                $subscriptions[] = new ExistingSubscriptionData(
                    feedUrl: $feedUrl,
                    title: is_string($title) && $title !== '' ? $title : null,
                );
            }

            $cursor = $response->json('cursor');
        } while (is_string($cursor) && $cursor !== '');

        return ExistingSubscriptionData::collect($subscriptions, DataCollection::class);
    }

    public function create(User $user, ChosenFeedData $choice): SubscriptionResultData
    {
        $response = $user->bluesky()
            ->client(auth: true)
            ->createRecord(
                repo: $user->did,
                collection: self::COLLECTION,
                record: $this->buildRecord($choice),
            );

        if ($response->failed()) {
            throw new RuntimeException("PDS write failed: {$response->status()} {$response->body()}");
        }

        return new SubscriptionResultData(
            title: $choice->title ?? $choice->feedUrl,
            atUri: $response->json('uri'),
        );
    }

    /**
     * @return array<string, mixed>
     */
    private function buildRecord(ChosenFeedData $choice): array
    {
        return array_filter([
            'feedUrl' => $choice->feedUrl,
            'title' => $choice->title,
            'siteUrl' => $choice->siteUrl,
            'sourceType' => $choice->sourceType->value,
            'createdAt' => Date::now()->toIso8601String(),
        ], fn ($value): bool => $value !== null);
    }
}
