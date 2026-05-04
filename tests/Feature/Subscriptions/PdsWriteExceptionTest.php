<?php

use App\Data\Feeds\ChosenFeedData;
use App\Exceptions\PdsWriteException;
use App\Models\User;
use App\Services\Subscriptions\SubscriptionService;
use Illuminate\Support\Facades\Http;

beforeEach(function () {
    Http::preventStrayRequests();
});

test('PdsWriteException carries the parsed status and errorCode', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('createRecord')
        ->andReturn($this->fakeFailureResponse(400, [
            'error' => 'InvalidRecord',
            'message' => 'Record validation failed',
        ]));

    $user = User::factory()->create();

    try {
        app(SubscriptionService::class)->create(
            $user,
            ChosenFeedData::from([
                'feed_url' => 'https://example.com/rss.xml',
                'title' => 'X',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ]),
        );
        expect()->fail('Expected PdsWriteException');
    } catch (PdsWriteException $e) {
        expect($e->status)->toBe(400)
            ->and($e->errorCode)->toBe('InvalidRecord')
            ->and($e->getMessage())->toContain('Record validation failed')
            ->and($e->collection)->toBe('app.skyreader.feed.subscription');
    }
});

test('a 200 response with a malformed at-uri throws PdsWriteException', function () {
    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('createRecord')
        ->andReturn($this->fakeSuccessResponse([
            'uri' => 'https://wrong-protocol.example/',
            'cid' => 'bafy...',
        ]));

    $user = User::factory()->create();

    try {
        app(SubscriptionService::class)->create(
            $user,
            ChosenFeedData::from([
                'feed_url' => 'https://example.com/rss.xml',
                'title' => 'X',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ]),
        );
        expect()->fail('Expected PdsWriteException');
    } catch (PdsWriteException $e) {
        expect($e->errorCode)->toBe('InvalidResponse');
    }
});
