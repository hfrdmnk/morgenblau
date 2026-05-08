<?php

namespace App\Services\Feeds;

use App\Data\Feeds\FeedEnclosureData;
use App\Data\Feeds\FetchedEntryData;
use App\Exceptions\FeedFetchException;
use App\Services\Feeds\Results\Failed;
use App\Services\Feeds\Results\FetchedFeedResult;
use App\Services\Feeds\Results\Gone;
use App\Services\Feeds\Results\Modified;
use App\Services\Feeds\Results\NotModified;
use App\Services\Feeds\Results\RateLimited;
use Carbon\CarbonImmutable;
use FeedIo\Adapter\NotFoundException;
use FeedIo\Adapter\ResponseInterface;
use FeedIo\Adapter\ServerErrorException;
use FeedIo\Feed;
use FeedIo\Feed\Item\MediaInterface;
use FeedIo\Feed\ItemInterface;
use FeedIo\FeedIo;
use Psr\Http\Message\ResponseInterface as PsrResponseInterface;
use Throwable;

class FeedFetcher
{
    public function __construct(
        private readonly FeedIo $feedIo,
        private readonly ConditionalFeedClient $client,
    ) {}

    public function fetch(string $feedUrl, ?string $etag = null, ?string $lastModified = null): FetchedFeedResult
    {
        try {
            $response = $this->client->fetchConditional($feedUrl, $etag, $lastModified);
        } catch (NotFoundException $e) {
            return new Failed(new FeedFetchException("HTTP 404 for {$feedUrl}", previous: $e));
        } catch (ServerErrorException $e) {
            return $this->mapServerError($feedUrl, $e);
        } catch (Throwable $e) {
            return new Failed(new FeedFetchException("Failed to fetch {$feedUrl}: {$e->getMessage()}", previous: $e));
        }

        $newEtag = $this->firstHeader($response, 'ETag');
        $newLastModified = $this->firstHeader($response, 'Last-Modified');

        if ($response->getStatusCode() === 304) {
            return new NotModified(etag: $newEtag, lastModified: $newLastModified);
        }

        try {
            $feed = new Feed;
            $feed->setLink($feedUrl);
            $this->feedIo->getReader()->handleResponse($response, $feed);
        } catch (Throwable $e) {
            return new Failed(new FeedFetchException("Failed to parse {$feedUrl}: {$e->getMessage()}", previous: $e));
        }

        $entries = [];
        foreach ($feed as $item) {
            if ($item instanceof ItemInterface) {
                $entries[] = $this->toEntry($item);
            }
        }

        return new Modified(entries: $entries, etag: $newEtag, lastModified: $newLastModified);
    }

    private function mapServerError(string $feedUrl, ServerErrorException $e): FetchedFeedResult
    {
        $response = $e->getResponse();
        $status = $response->getStatusCode();

        if ($status === 410) {
            return new Gone;
        }

        if ($status === 429) {
            return new RateLimited($this->parseRetryAfter($response));
        }

        return new Failed(new FeedFetchException("HTTP {$status} for {$feedUrl}", previous: $e));
    }

    private function parseRetryAfter(PsrResponseInterface $response): int
    {
        $header = trim($response->getHeaderLine('Retry-After'));
        if ($header === '') {
            return 0;
        }

        if (ctype_digit($header)) {
            return (int) $header;
        }

        $timestamp = strtotime($header);
        if ($timestamp === false) {
            return 0;
        }

        return max(0, $timestamp - time());
    }

    private function toEntry(ItemInterface $item): FetchedEntryData
    {
        $lastModified = $item->getLastModified();
        $link = $item->getLink();
        $guid = $item->getPublicId() ?? $link;

        return new FetchedEntryData(
            title: $item->getTitle(),
            link: $link,
            guid: $guid,
            summary: $item->getSummary(),
            content: $item->getContent(),
            author: $item->getAuthor()?->getName(),
            publishedAt: $lastModified !== null ? CarbonImmutable::instance($lastModified) : null,
            enclosures: $this->toEnclosures($item),
        );
    }

    /**
     * @return list<FeedEnclosureData>|null
     */
    private function toEnclosures(ItemInterface $item): ?array
    {
        if (! $item->hasMedia()) {
            return null;
        }

        $out = [];
        foreach ($item->getMedias() as $media) {
            if (! $media instanceof MediaInterface) {
                continue;
            }
            $url = $media->getUrl();
            if ($url === null || $url === '') {
                continue;
            }
            $length = $media->getLength();
            $out[] = new FeedEnclosureData(
                url: $url,
                type: $media->getType(),
                length: $length !== null && $length !== '' && ctype_digit((string) $length) ? (int) $length : null,
            );
        }

        return $out === [] ? null : $out;
    }

    private function firstHeader(ResponseInterface $response, string $name): ?string
    {
        foreach ($response->getHeader($name) as $value) {
            $value = trim((string) $value);
            if ($value !== '') {
                return $value;
            }
        }

        return null;
    }
}
