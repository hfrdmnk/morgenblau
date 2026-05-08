<?php

namespace App\Services\Feeds;

use App\Data\Feeds\FetchedEntryData;
use App\Exceptions\FeedFetchException;
use App\Services\Feeds\Results\FetchedFeedResult;
use App\Services\Feeds\Results\Modified;
use App\Services\Feeds\Results\NotModified;
use Carbon\CarbonImmutable;
use FeedIo\Adapter\ResponseInterface;
use FeedIo\Feed;
use FeedIo\Feed\ItemInterface;
use FeedIo\FeedIo;
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
        } catch (Throwable $e) {
            throw new FeedFetchException("Failed to fetch {$feedUrl}: {$e->getMessage()}", previous: $e);
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
            throw new FeedFetchException("Failed to parse {$feedUrl}: {$e->getMessage()}", previous: $e);
        }

        $entries = [];
        foreach ($feed as $item) {
            if ($item instanceof ItemInterface) {
                $entries[] = $this->toEntry($item);
            }
        }

        return new Modified(entries: $entries, etag: $newEtag, lastModified: $newLastModified);
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
        );
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
