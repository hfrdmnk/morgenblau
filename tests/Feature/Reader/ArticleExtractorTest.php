<?php

use App\Enums\Reader\ExtractionFailureReason;
use App\Services\Http\DnsResolver;
use App\Services\Reader\ArticleExtractor;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Support\Facades\Http;
use Tests\Doubles\FakeDnsResolver;

beforeEach(function () {
    Http::preventStrayRequests();
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver([
        'substack.example' => ['93.184.216.34'],
        'blog.example' => ['93.184.216.34'],
        'news.example' => ['93.184.216.34'],
        'mal.example' => ['93.184.216.34'],
        'gone.example' => ['93.184.216.34'],
        'slow.example' => ['93.184.216.34'],
        'empty.example' => ['93.184.216.34'],
    ]));
});

function fixtureHtml(string $name): string
{
    return (string) file_get_contents(__DIR__.'/fixtures/'.$name);
}

test('extracts a Substack-style article', function () {
    Http::fake([
        'substack.example/*' => Http::response(fixtureHtml('substack.html'), 200, ['Content-Type' => 'text/html']),
    ]);

    $result = app(ArticleExtractor::class)->extract('https://substack.example/p/quiet-software');

    expect($result->isSuccess())->toBeTrue();
    expect($result->html)->toContain('calm');
    expect($result->title)->toContain('Quiet Software');
    expect($result->wordCount)->toBeGreaterThan(200);
    expect($result->readingTimeSeconds)->toBeGreaterThan(0);
});

test('extracts a blog-style article', function () {
    Http::fake([
        'blog.example/*' => Http::response(fixtureHtml('blog.html'), 200, ['Content-Type' => 'text/html']),
    ]);

    $result = app(ArticleExtractor::class)->extract('https://blog.example/posts/slow-reading');

    expect($result->isSuccess())->toBeTrue();
    expect($result->html)->toContain('slow');
    expect($result->wordCount)->toBeGreaterThan(200);
});

test('extracts a news-style article', function () {
    Http::fake([
        'news.example/*' => Http::response(fixtureHtml('news.html'), 200, ['Content-Type' => 'text/html']),
    ]);

    $result = app(ArticleExtractor::class)->extract('https://news.example/article/library-slow-reading');

    expect($result->isSuccess())->toBeTrue();
    expect($result->html)->toContain('library');
    expect($result->wordCount)->toBeGreaterThan(200);
});

test('sanitises script tags and inline event handlers from extractor output', function () {
    Http::fake([
        'mal.example/*' => Http::response(fixtureHtml('malicious.html'), 200, ['Content-Type' => 'text/html']),
    ]);

    $result = app(ArticleExtractor::class)->extract('https://mal.example/post');

    expect($result->isSuccess())->toBeTrue();
    expect($result->html)
        ->not->toContain('<script')
        ->not->toContain('onclick')
        ->not->toContain('onmouseover')
        ->not->toContain('javascript:');
});

test('returns Unreachable on a 404 response', function () {
    Http::fake([
        'gone.example/*' => Http::response('Not found', 404),
    ]);

    $result = app(ArticleExtractor::class)->extract('https://gone.example/post');

    expect($result->isSuccess())->toBeFalse();
    expect($result->failureReason)->toBe(ExtractionFailureReason::Unreachable);
});

test('returns Unreachable on a connection timeout', function () {
    Http::fake(function () {
        throw new ConnectionException('cURL timeout');
    });

    $result = app(ArticleExtractor::class)->extract('https://slow.example/post');

    expect($result->isSuccess())->toBeFalse();
    expect($result->failureReason)->toBe(ExtractionFailureReason::Unreachable);
});

test('returns NoContent when readability cannot find an article body', function () {
    Http::fake([
        'empty.example/*' => Http::response('<!doctype html><html><head><title></title></head><body></body></html>', 200, ['Content-Type' => 'text/html']),
    ]);

    $result = app(ArticleExtractor::class)->extract('https://empty.example/post');

    expect($result->isSuccess())->toBeFalse();
    expect($result->failureReason)->toBe(ExtractionFailureReason::NoContent);
});
