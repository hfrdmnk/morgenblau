<?php

namespace App\Services\Feeds\FeedIo;

use DateTime;
use DOMDocument;
use DOMElement;
use FeedIo\Feed\NodeInterface;
use FeedIo\Rule\ModifiedSince;

/**
 * Atom rule that prefers <published> over <updated> for the entry's date.
 *
 * feed-io's default Atom standard maps both tags to setLastModified() and
 * lets XML order decide which wins. YouTube ships <updated> after <published>
 * and uses it for view-count refresh timestamps, so the default behavior
 * stores YouTube's "last touched" date as our publish date. We override:
 * when <published> exists on the entry, <updated> is ignored.
 */
class AtomPublishedDate extends ModifiedSince
{
    public function setProperty(NodeInterface $node, DOMElement $element): void
    {
        $tag = strtolower($element->localName ?? $element->tagName);

        if ($tag !== 'published' && $this->hasPublishedSibling($element)) {
            return;
        }

        $node->setLastModified($this->getDateTimeBuilder()->convertToDateTime($element->nodeValue));
    }

    protected function addElement(DOMDocument $document, DOMElement $rootElement, NodeInterface $node): void
    {
        $date = $node->getLastModified() ?? new DateTime;

        $rootElement->appendChild($document->createElement(
            $this->getNodeName(),
            $date->format($this->getDefaultFormat()),
        ));
    }

    private function hasPublishedSibling(DOMElement $element): bool
    {
        $parent = $element->parentNode;
        if ($parent === null) {
            return false;
        }

        foreach ($parent->childNodes as $child) {
            if ($child instanceof DOMElement
                && strtolower($child->localName ?? $child->tagName) === 'published') {
                return true;
            }
        }

        return false;
    }
}
