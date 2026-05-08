<?php

namespace App\Services\Feeds\Processors;

use App\Data\Feeds\FetchedEntryData;
use App\Data\Feeds\ProcessedEntryData;
use App\Models\Feed;

class ProcessorPipeline
{
    /** @var list<EntryProcessor> */
    private array $processors;

    /**
     * @param  iterable<EntryProcessor>  $processors
     */
    public function __construct(iterable $processors)
    {
        $this->processors = is_array($processors) ? array_values($processors) : iterator_to_array($processors, false);
    }

    /**
     * @param  iterable<FetchedEntryData>  $entries
     * @return list<ProcessedEntryData>
     */
    public function processBatch(iterable $entries, Feed $feed): array
    {
        $out = [];
        foreach ($entries as $entry) {
            $processed = ProcessedEntryData::fromFetched($entry);
            foreach ($this->processors as $processor) {
                $processed = $processor->process($processed, $feed);
            }
            $out[] = $processed;
        }

        return $out;
    }
}
