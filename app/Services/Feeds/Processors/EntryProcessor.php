<?php

namespace App\Services\Feeds\Processors;

use App\Data\Feeds\ProcessedEntryData;
use App\Models\Feed;

interface EntryProcessor
{
    public function process(ProcessedEntryData $entry, Feed $feed): ProcessedEntryData;
}
