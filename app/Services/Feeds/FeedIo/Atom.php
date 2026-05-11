<?php

namespace App\Services\Feeds\FeedIo;

use FeedIo\RuleSet;
use FeedIo\Standard\Atom as BaseAtom;

class Atom extends BaseAtom
{
    protected function buildBaseRuleSet(): RuleSet
    {
        $ruleSet = parent::buildBaseRuleSet();

        $rule = new AtomPublishedDate('updated');
        $rule->setDateTimeBuilder($this->dateTimeBuilder);
        $rule->setDefaultFormat($this->getDefaultDateFormat());

        return $ruleSet->add($rule, ['published']);
    }
}
