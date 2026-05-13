<?php

namespace App\Services\Feeds\FeedIo;

use FeedIo\RuleSet;
use FeedIo\Standard\Rss as BaseRss;

class Rss extends BaseRss
{
    public function buildItemRuleSet(): RuleSet
    {
        return parent::buildItemRuleSet()
            ->add(new Description)
            ->add(new ContentEncoded);
    }
}
