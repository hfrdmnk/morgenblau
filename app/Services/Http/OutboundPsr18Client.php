<?php

namespace App\Services\Http;

use Psr\Http\Client\ClientInterface;
use Psr\Http\Client\NetworkExceptionInterface;
use Psr\Http\Message\RequestInterface;
use Psr\Http\Message\ResponseInterface;
use RuntimeException;
use Throwable;

/**
 * PSR-18 facade over OutboundHttpClient. Lets PSR-18-only libraries (e.g.
 * php-feed-io/favicon-io) inherit our SSRF guard, redirect re-validation,
 * and 5 MB body cap without bringing in their own HTTP stack.
 *
 * Untrusted-URL path only — favicon discovery targets arbitrary user feed
 * hosts, so getTrusted() has no role here.
 */
class OutboundPsr18Client implements ClientInterface
{
    public function __construct(private readonly OutboundHttpClient $http) {}

    public function sendRequest(RequestInterface $request): ResponseInterface
    {
        $headers = [];
        foreach ($request->getHeaders() as $name => $values) {
            $headers[$name] = implode(', ', $values);
        }

        try {
            $response = $this->http->sendUserUrl(
                $request->getMethod(),
                (string) $request->getUri(),
                $headers,
            );
        } catch (Throwable $e) {
            throw new class($e->getMessage(), $request, $e) extends RuntimeException implements NetworkExceptionInterface
            {
                public function __construct(string $message, private readonly RequestInterface $request, Throwable $previous)
                {
                    parent::__construct($message, 0, $previous);
                }

                public function getRequest(): RequestInterface
                {
                    return $this->request;
                }
            };
        }

        return $response->toPsrResponse();
    }
}
