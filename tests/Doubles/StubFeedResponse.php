<?php

namespace Tests\Doubles;

use DateTime;
use FeedIo\Adapter\ResponseInterface;

class StubFeedResponse implements ResponseInterface
{
    public function __construct(
        private readonly string $body,
        private readonly int $status,
        private readonly ?string $etag,
        private readonly ?string $lastModified,
    ) {}

    public function getBody(): ?string
    {
        return $this->body;
    }

    public function getStatusCode(): int
    {
        return $this->status;
    }

    public function getDuration(): float
    {
        return 0.0;
    }

    public function isModified(): bool
    {
        return $this->status !== 304 && strlen($this->body) > 0;
    }

    public function getLastModified(): ?DateTime
    {
        if ($this->lastModified === null) {
            return null;
        }

        $parsed = DateTime::createFromFormat(DateTime::RFC2822, $this->lastModified);

        return $parsed === false ? null : $parsed;
    }

    public function getHeaders(): iterable
    {
        $headers = [];
        if ($this->etag !== null) {
            $headers['ETag'] = [$this->etag];
        }
        if ($this->lastModified !== null) {
            $headers['Last-Modified'] = [$this->lastModified];
        }

        return $headers;
    }

    public function getHeader(string $name): iterable
    {
        $lower = strtolower($name);
        if ($lower === 'etag' && $this->etag !== null) {
            return [$this->etag];
        }
        if ($lower === 'last-modified' && $this->lastModified !== null) {
            return [$this->lastModified];
        }

        return [];
    }
}
