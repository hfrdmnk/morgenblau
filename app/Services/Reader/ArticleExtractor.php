<?php

namespace App\Services\Reader;

use App\Data\Reader\ExtractionResult;
use App\Enums\Reader\ExtractionFailureReason;
use App\Exceptions\UnsafeUrlException;
use App\Services\Http\OutboundHttpClient;
use fivefilters\Readability\Configuration;
use fivefilters\Readability\ParseException;
use fivefilters\Readability\Readability;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Http\Client\RequestException;
use Stevebauman\Purify\Facades\Purify;
use Throwable;

/**
 * Deep module: fetch a source URL, run readability, sanitize the result, and
 * classify any failure. Callers see a single ExtractionResult — they don't
 * need to know whether failure came from HTTP, parsing, or empty content.
 *
 * Reusable by both the queued auto path (ExtractArticleJob) and any future
 * synchronous manual-fetch endpoint (Slice 4): extract() is pure with respect
 * to entry state — the caller decides what to persist and how.
 */
class ArticleExtractor
{
    /**
     * Match common desktop UA. Many publishers serve a 403 or a stripped
     * page to bare Guzzle UA strings; this one mirrors typical RSS readers.
     */
    private const USER_AGENT = 'Mozilla/5.0 (compatible; MorgenblauReader/1.0; +https://morgen.blue)';

    private const READING_WPM = 220;

    public function __construct(private readonly OutboundHttpClient $http) {}

    public function extract(string $url): ExtractionResult
    {
        try {
            $response = $this->http->getUserUrl($url, ['User-Agent' => self::USER_AGENT]);
        } catch (UnsafeUrlException|ConnectionException|RequestException|Throwable) {
            return ExtractionResult::failure(ExtractionFailureReason::Unreachable);
        }

        if (! $response->successful()) {
            return ExtractionResult::failure(ExtractionFailureReason::Unreachable);
        }

        $body = (string) $response->body();
        if ($body === '') {
            return ExtractionResult::failure(ExtractionFailureReason::NoContent);
        }

        $configuration = new Configuration([
            'fixRelativeURLs' => true,
            'originalURL' => $url,
        ]);

        $readability = new Readability($configuration);

        try {
            $readability->parse($body);
        } catch (ParseException) {
            return ExtractionResult::failure(ExtractionFailureReason::NoContent);
        }

        $content = $readability->getContent();
        if ($content === null || trim($content) === '') {
            return ExtractionResult::failure(ExtractionFailureReason::NoContent);
        }

        $sanitized = (string) Purify::config('blogpost')->clean($content);
        if (trim(strip_tags($sanitized)) === '') {
            return ExtractionResult::failure(ExtractionFailureReason::NoContent);
        }

        $words = self::wordCount($sanitized);

        return ExtractionResult::success(
            html: $sanitized,
            title: $readability->getTitle(),
            author: $readability->getAuthor(),
            imageUrl: $readability->getImage(),
            wordCount: $words,
            readingTimeSeconds: (int) max(1, ceil($words / self::READING_WPM * 60)),
        );
    }

    private static function wordCount(string $html): int
    {
        $text = trim(strip_tags($html));
        if ($text === '') {
            return 0;
        }

        return str_word_count(preg_replace('/\s+/u', ' ', $text) ?? $text);
    }
}
