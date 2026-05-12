<?php

namespace App\Data\Reader;

use App\Enums\Reader\ExtractionFailureReason;
use Spatie\LaravelData\Data;

/**
 * Internal DTO for the ArticleExtractor service. Two shapes:
 *   - success: html + metadata, failureReason is null
 *   - failure: failureReason is set, body fields are null
 * Construct via the static factories rather than the constructor.
 */
class ExtractionResult extends Data
{
    public function __construct(
        public readonly ?string $html,
        public readonly ?string $title,
        public readonly ?string $author,
        public readonly ?string $imageUrl,
        public readonly ?int $wordCount,
        public readonly ?int $readingTimeSeconds,
        public readonly ?ExtractionFailureReason $failureReason,
    ) {}

    public static function success(
        string $html,
        ?string $title,
        ?string $author,
        ?string $imageUrl,
        int $wordCount,
        int $readingTimeSeconds,
    ): self {
        return new self(
            html: $html,
            title: $title,
            author: $author,
            imageUrl: $imageUrl,
            wordCount: $wordCount,
            readingTimeSeconds: $readingTimeSeconds,
            failureReason: null,
        );
    }

    public static function failure(ExtractionFailureReason $reason): self
    {
        return new self(
            html: null,
            title: null,
            author: null,
            imageUrl: null,
            wordCount: null,
            readingTimeSeconds: null,
            failureReason: $reason,
        );
    }

    public function isSuccess(): bool
    {
        return $this->failureReason === null;
    }
}
