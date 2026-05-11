<?php

namespace App\Services\Feeds\FeedIo;

use FeedIo\Specification as BaseSpecification;
use FeedIo\Standard\Json;
use FeedIo\Standard\Rdf;
use FeedIo\Standard\Rss;

class Specification extends BaseSpecification
{
    protected function getDefaultStandards(): array
    {
        return [
            'json' => new Json($this->dateTimeBuilder),
            'atom' => new Atom($this->dateTimeBuilder),
            'rss' => new Rss($this->dateTimeBuilder),
            'rdf' => new Rdf($this->dateTimeBuilder),
        ];
    }
}
