<?php

namespace Tests\Concerns;

use App\Models\User;
use Illuminate\Auth\AuthenticationException;
use Illuminate\Http\Client\Response as HttpResponse;
use Illuminate\Support\Facades\Http;
use Mockery;
use Mockery\MockInterface;
use Revolution\Bluesky\Client\AtpClient;
use Revolution\Bluesky\Contracts\Factory as BlueskyFactory;

trait FakesBlueskyClient
{
    /**
     * Build a fully-faked factory + client and bind it. Use when the test
     * doesn't care about refresh-session expectations.
     */
    protected function fakeBlueskyClient(): MockInterface
    {
        $client = Mockery::mock(AtpClient::class);

        $factory = Mockery::mock(BlueskyFactory::class);
        $factory->shouldReceive('withToken')->andReturnSelf();
        $factory->shouldReceive('refreshSession')->andReturnSelf();
        $factory->shouldReceive('client')->with(true)->andReturn($client);

        app()->instance(BlueskyFactory::class, $factory);

        return $client;
    }

    /**
     * Bolt a client onto an existing factory mock (built by blueskyFactoryMock()
     * in tests/Pest.php). Use when the test needs to control refreshSession
     * itself.
     */
    protected function fakeBlueskyClientOn(MockInterface $factory): MockInterface
    {
        $client = Mockery::mock(AtpClient::class);
        $factory->shouldReceive('client')->with(true)->andReturn($client);

        return $client;
    }

    protected function fakeBlueskyRefreshFailure(): void
    {
        $factory = Mockery::mock(BlueskyFactory::class);
        $factory->shouldReceive('withToken')->andReturnSelf();
        $factory->shouldReceive('refreshSession')->andThrow(new AuthenticationException);

        app()->instance(BlueskyFactory::class, $factory);
    }

    /**
     * @param  array<int, string|array{feed_url: string, title?: ?string}>  $records
     */
    protected function fakeListRecords(MockInterface $client, array $records = []): void
    {
        $client->shouldReceive('listRecords')
            ->andReturn($this->fakeSuccessResponse([
                'records' => array_map(function ($record) {
                    if (is_string($record)) {
                        return ['value' => ['feedUrl' => $record]];
                    }

                    return ['value' => array_filter([
                        'feedUrl' => $record['feed_url'],
                        'title' => $record['title'] ?? null,
                    ], fn ($value): bool => $value !== null)];
                }, $records),
            ]));
    }

    /**
     * @param  array<int, string|array{feed_url: string, title?: ?string}>  $records
     */
    protected function fakeListRecordsOnFactory(MockInterface $factory, array $records = []): void
    {
        $this->fakeListRecords($this->fakeBlueskyClientOn($factory), $records);
    }

    /**
     * @param  array<string, mixed>  $body
     */
    protected function fakeSuccessResponse(array $body): HttpResponse
    {
        return new HttpResponse(Http::response($body, 200)->wait());
    }

    /**
     * @param  array<string, mixed>  $body
     */
    protected function fakeFailureResponse(int $status, array $body = []): HttpResponse
    {
        return new HttpResponse(Http::response($body, $status)->wait());
    }

    protected function fakeHtmlResponse(string $host = 'example.com'): void
    {
        Http::fake([
            $host => Http::response(
                '<html><head><link rel="alternate" type="application/rss+xml" title="Main" href="/rss.xml"></head></html>',
                200,
                ['Content-Type' => 'text/html'],
            ),
        ]);
    }

    protected function userWithBlueskySession(string $accessToken): User
    {
        $user = User::factory()->create([
            'iss' => 'https://eurosky.social',
        ]);

        session()->put('bluesky_session', [
            'did' => $user->did,
            'access_token' => $accessToken,
            'refresh_token' => $user->refresh_token,
            'iss' => $user->iss,
        ]);

        return $user;
    }
}
