<?php

namespace Tests\Doubles;

use App\Services\Feeds\ConditionalFeedClient;
use DateTime;
use FeedIo\Adapter\ClientInterface;
use FeedIo\Adapter\NotFoundException;
use FeedIo\Adapter\ResponseInterface;

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
     * @param  array<string, string|array{result?: 'modified'|'not_modified', body?: string, etag?: ?string, last_modified?: ?string}>  $responses
     *                                                                                                                                              A bare string is shorthand for ['result' => 'modified', 'body' => $string].
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

        $status = $config['result'] === 'not_modified' ? 304 : 200;
        $body = $config['result'] === 'not_modified' ? '' : (string) $config['body'];

        return new StubFeedResponse(
            body: $body,
            status: $status,
            etag: $config['etag'] ?? null,
            lastModified: $config['last_modified'] ?? null,
        );
    }

    /**
     * @param  string|array<string, mixed>  $config
     * @return array{result: 'modified'|'not_modified', body: ?string, etag: ?string, last_modified: ?string}
     */
    private function normalize(string|array $config): array
    {
        if (is_string($config)) {
            return ['result' => 'modified', 'body' => $config, 'etag' => null, 'last_modified' => null];
        }

        return [
            'result' => $config['result'] ?? 'modified',
            'body' => $config['body'] ?? null,
            'etag' => $config['etag'] ?? null,
            'last_modified' => $config['last_modified'] ?? null,
        ];
    }
}
