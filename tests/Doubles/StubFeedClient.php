<?php

namespace Tests\Doubles;

use DateTime;
use FeedIo\Adapter\ClientInterface;
use FeedIo\Adapter\NotFoundException;
use FeedIo\Adapter\ResponseInterface;

class StubFeedClient implements ClientInterface
{
    /**
     * @param  array<string, string>  $bodies  url => xml body
     */
    public function __construct(private readonly array $bodies) {}

    public function getResponse(string $url, ?DateTime $modifiedSince = null): ResponseInterface
    {
        if (! isset($this->bodies[$url])) {
            throw new NotFoundException("No fixture mapped for {$url}.");
        }

        return new class($this->bodies[$url]) implements ResponseInterface
        {
            public function __construct(private readonly string $body) {}

            public function getBody(): ?string
            {
                return $this->body;
            }

            public function getStatusCode(): int
            {
                return 200;
            }

            public function getDuration(): float
            {
                return 0.0;
            }

            public function isModified(): bool
            {
                return true;
            }

            public function getLastModified(): ?DateTime
            {
                return null;
            }

            public function getHeaders(): iterable
            {
                return [];
            }

            public function getHeader(string $name): iterable
            {
                return [];
            }
        };
    }
}
