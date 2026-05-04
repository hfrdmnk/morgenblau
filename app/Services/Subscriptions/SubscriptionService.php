<?php

namespace App\Services\Subscriptions;

use App\Data\Feeds\ChosenFeedData;
use App\Data\Feeds\ResolvedFeedData;
use App\Data\Subscriptions\ExistingSubscriptionData;
use App\Data\Subscriptions\SubscriptionResultData;
use App\Exceptions\AlreadySubscribedException;
use App\Exceptions\PdsReadException;
use App\Exceptions\PdsWriteException;
use App\Models\User;
use App\Services\Feeds\FeedResolver;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\Log;
use Spatie\LaravelData\DataCollection;
use Throwable;

class SubscriptionService
{
    private const COLLECTION = 'app.skyreader.feed.subscription';

    /** Per-user cache TTL for listSubscriptions, in seconds. */
    private const LIST_CACHE_TTL = 30;

    /** Safety cap on PDS pagination. */
    private const MAX_PAGES = 50;

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
        return Cache::remember(
            $this->listCacheKey($user),
            self::LIST_CACHE_TTL,
            fn (): DataCollection => $this->fetchSubscriptionsFromPds($user),
        );
    }

    /**
     * Batch-create subscriptions under a per-user write lock so concurrent
     * submits can't race past dedup. Cross-app entries (records written by
     * other apps with their own rkey schemes) are still caught here because
     * dedup runs on the feedUrl *value*, not on the rkey.
     *
     * @param  iterable<ChosenFeedData>  $choices
     * @return array{0: list<SubscriptionResultData>, 1: list<array{choice: ChosenFeedData, error: Throwable}>}
     */
    public function createMany(User $user, iterable $choices): array
    {
        return Cache::lock("subscriptions:write:{$user->did}", 30)->block(8, function () use ($user, $choices): array {
            // Bypass-and-warm so dedup runs against authoritative state.
            Cache::forget($this->listCacheKey($user));
            $existing = array_flip(
                $this->listSubscriptions($user)->toCollection()->pluck('feedUrl')->all(),
            );

            $succeeded = [];
            $failed = [];

            foreach ($choices as $choice) {
                if (isset($existing[$choice->feedUrl])) {
                    $failed[] = [
                        'choice' => $choice,
                        'error' => new AlreadySubscribedException($choice->feedUrl),
                    ];

                    continue;
                }

                try {
                    $result = $this->create($user, $choice);
                    $existing[$choice->feedUrl] = true;
                    $succeeded[] = $result;
                } catch (PdsWriteException $e) {
                    $failed[] = ['choice' => $choice, 'error' => $e];
                }
            }

            // Invalidate again so the next listSubscriptions reflects the new entries.
            Cache::forget($this->listCacheKey($user));

            return [$succeeded, $failed];
        });
    }

    public function create(User $user, ChosenFeedData $choice): SubscriptionResultData
    {
        // TODO: pass validate: true once skyreader lexicons are published as com.atproto.lexicon.schema records.
        $response = $user->bluesky()
            ->client(auth: true)
            ->createRecord(
                repo: $user->did,
                collection: self::COLLECTION,
                record: $this->buildRecord($choice),
            );

        if ($response->failed()) {
            throw PdsWriteException::fromResponse(self::COLLECTION, $response);
        }

        $uri = $response->json('uri');
        if (! is_string($uri) || ! str_starts_with($uri, 'at://')) {
            throw new PdsWriteException(
                collection: self::COLLECTION,
                status: $response->status(),
                errorCode: 'InvalidResponse',
                message: 'PDS returned a malformed at-uri.',
            );
        }

        return new SubscriptionResultData(
            title: $choice->title ?? $choice->feedUrl,
            atUri: $uri,
        );
    }

    private function fetchSubscriptionsFromPds(User $user): DataCollection
    {
        $client = $user->bluesky()->client(auth: true);

        $subscriptions = [];
        $cursor = null;
        $pages = 0;

        do {
            if ($pages++ >= self::MAX_PAGES) {
                Log::warning('subscriptions pagination cap hit', [
                    'did' => $user->did,
                    'cursor' => $cursor,
                ]);
                break;
            }

            $response = $client->listRecords(
                repo: $user->did,
                collection: self::COLLECTION,
                limit: 100,
                cursor: $cursor,
            );

            if ($response->failed()) {
                throw PdsReadException::fromResponse(self::COLLECTION, $response);
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

    private function listCacheKey(User $user): string
    {
        return "subscriptions:list:{$user->did}";
    }

    /**
     * @return array<string, mixed>
     */
    private function buildRecord(ChosenFeedData $choice): array
    {
        return array_filter([
            '$type' => self::COLLECTION,
            'feedUrl' => $choice->feedUrl,
            'title' => $choice->title,
            'siteUrl' => $choice->siteUrl,
            'sourceType' => $choice->sourceType->value,
            'createdAt' => Date::now()->utc()->format('Y-m-d\\TH:i:s.u\\Z'),
        ], fn ($value): bool => $value !== null);
    }
}
