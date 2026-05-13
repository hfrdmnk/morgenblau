<?php

namespace App\Services\Feeds\FeedIo;

use DomDocument;
use DOMElement;
use FeedIo\Feed\ItemInterface;
use FeedIo\Feed\NodeInterface;
use FeedIo\Rule\TextAbstract;

/**
 * RSS <description> → ItemInterface::summary (Atom-equivalent semantics).
 * Also writes to content when content isn't yet set, preserving the stock
 * behaviour for plain RSS feeds without <content:encoded>; ContentEncoded
 * will overwrite content later when it fires.
 */
class Description extends TextAbstract
{
    public const NODE_NAME = 'description';

    public function setProperty(NodeInterface $node, DOMElement $element): void
    {
        if (! $node instanceof ItemInterface) {
            return;
        }

        $value = $this->getProcessedContent($element, $node);
        if (trim($value) === '') {
            return;
        }

        $node->setSummary($value);

        if (! $node->getContent()) {
            $node->setContent($value);
        }
    }

    protected function hasValue(NodeInterface $node): bool
    {
        return $node instanceof ItemInterface && (bool) $node->getSummary();
    }

    protected function addElement(DomDocument $document, DOMElement $rootElement, NodeInterface $node): void
    {
        // Read-only: we never serialise <description> back out.
    }
}
