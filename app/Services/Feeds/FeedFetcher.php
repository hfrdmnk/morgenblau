<?php

namespace App\Services\Feeds;

use App\Data\Feeds\FetchedEntryData;
use App\Exceptions\FeedFetchException;
use Carbon\CarbonImmutable;
use FeedIo\Feed\ItemInterface;
use FeedIo\FeedIo;
use Throwable;

class FeedFetcher
{
    public function __construct(private readonly FeedIo $feedIo) {}

    /**
     * @return list<FetchedEntryData>
     */
    public function fetch(string $feedUrl): array
    {
        try {
            $result = $this->feedIo->read($feedUrl);
        } catch (Throwable $e) {
            throw new FeedFetchException("Failed to fetch {$feedUrl}: {$e->getMessage()}", previous: $e);
        }

        $entries = [];
        foreach ($result->getFeed() as $item) {
            if ($item instanceof ItemInterface) {
                $entries[] = $this->toEntry($item);
            }
        }

        return $entries;
    }

    private function toEntry(ItemInterface $item): FetchedEntryData
    {
        $lastModified = $item->getLastModified();

        return new FetchedEntryData(
            title: $item->getTitle(),
            link: $item->getLink(),
            guid: $item->getPublicId(),
            summary: $item->getSummary(),
            content: $item->getContent(),
            author: $item->getAuthor()?->getName(),
            publishedAt: $lastModified !== null ? CarbonImmutable::instance($lastModified) : null,
        );
    }
}
