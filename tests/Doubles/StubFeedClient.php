<?php

namespace Tests\Doubles;

use App\Services\Feeds\ConditionalFeedClient;
use DateTime;
use FeedIo\Adapter\ClientInterface;
use FeedIo\Adapter\NotFoundException;
use FeedIo\Adapter\ResponseInterface;
use FeedIo\Adapter\ServerErrorException;
use GuzzleHttp\Psr7\Response as Psr7Response;

class StubFeedClient implements ClientInterface, ConditionalFeedClient
{
    /**
     * Records the headers passed on the most recent call per URL, so tests can
     * assert that conditional-GET headers were forwarded.
     *
     * @var array<string, array{etag: ?string, last_modified: ?string}>
     */
    public array $lastConditionalHeaders = [];

    /**
     * @param  array<string, string|array<string, mixed>>  $responses
     *                                                                 A bare string is shorthand for ['result' => 'modified', 'body' => $string].
     *                                                                 Supported shapes:
     *                                                                 ['result' => 'modified', 'body' => string, 'etag' => ?string, 'last_modified' => ?string]
     *                                                                 ['result' => 'not_modified', 'etag' => ?string, 'last_modified' => ?string]
     *                                                                 ['result' => 'gone']
     *                                                                 ['result' => 'rate_limited', 'retry_after' => int|string]
     *                                                                 ['result' => 'failed', 'status' => int]
     */
    public function __construct(private readonly array $responses) {}

    public function getResponse(string $url, ?DateTime $modifiedSince = null): ResponseInterface
    {
        $lastModified = $modifiedSince?->format(DateTime::RFC2822);

        return $this->respond($url, etag: null, lastModified: $lastModified);
    }

    public function fetchConditional(string $url, ?string $etag, ?string $lastModified): ResponseInterface
    {
        return $this->respond($url, $etag, $lastModified);
    }

    private function respond(string $url, ?string $etag, ?string $lastModified): ResponseInterface
    {
        if (! isset($this->responses[$url])) {
            throw new NotFoundException("No fixture mapped for {$url}.");
        }

        $this->lastConditionalHeaders[$url] = ['etag' => $etag, 'last_modified' => $lastModified];

        $config = $this->normalize($this->responses[$url]);
        $result = $config['result'];

        if ($result === 'gone') {
            throw new ServerErrorException(new Psr7Response(410));
        }

        if ($result === 'rate_limited') {
            $headers = [];
            if (isset($config['retry_after'])) {
                $headers['Retry-After'] = (string) $config['retry_after'];
            }
            throw new ServerErrorException(new Psr7Response(429, $headers));
        }

        if ($result === 'failed') {
            $status = (int) ($config['status'] ?? 500);
            if ($status === 404) {
                throw new NotFoundException('not found');
            }
            throw new ServerErrorException(new Psr7Response($status));
        }

        $status = $result === 'not_modified' ? 304 : 200;
        $body = $result === 'not_modified' ? '' : (string) ($config['body'] ?? '');

        return new StubFeedResponse(
            body: $body,
            status: $status,
            etag: $config['etag'] ?? null,
            lastModified: $config['last_modified'] ?? null,
        );
    }

    /**
     * @param  string|array<string, mixed>  $config
     * @return array<string, mixed>
     */
    private function normalize(string|array $config): array
    {
        if (is_string($config)) {
            return ['result' => 'modified', 'body' => $config];
        }

        return $config + ['result' => 'modified'];
    }
}
