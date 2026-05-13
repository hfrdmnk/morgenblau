<?php

namespace App\Services\Feeds\FeedIo;

use DomDocument;
use DOMElement;
use FeedIo\Feed\ItemInterface;
use FeedIo\Feed\NodeInterface;
use FeedIo\Rule\TextAbstract;

/** Maps <content:encoded> → ItemInterface::content; stock feed-io RSS reads only <description>. */
class ContentEncoded extends TextAbstract
{
    public const NODE_NAME = 'content:encoded';

    public function setProperty(NodeInterface $node, DOMElement $element): void
    {
        if (! $node instanceof ItemInterface) {
            return;
        }

        $value = $this->getProcessedContent($element, $node);
        if (trim($value) === '') {
            return;
        }

        $node->setContent($value);
    }

    protected function hasValue(NodeInterface $node): bool
    {
        return $node instanceof ItemInterface && (bool) $node->getContent();
    }

    protected function addElement(DomDocument $document, DOMElement $rootElement, NodeInterface $node): void
    {
        // Read-only: we never serialise content:encoded back out.
    }
}
