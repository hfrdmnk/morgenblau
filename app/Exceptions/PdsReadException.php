<?php

namespace App\Exceptions;

use Illuminate\Http\Client\Response;
use RuntimeException;
use Throwable;

class PdsReadException extends RuntimeException
{
    public function __construct(
        public readonly string $collection,
        public readonly int $status,
        public readonly ?string $errorCode,
        ?string $message = null,
        ?Throwable $previous = null,
    ) {
        parent::__construct(
            $message ?? "PDS read of {$collection} failed: {$status} {$errorCode}",
            previous: $previous,
        );
    }

    public static function fromResponse(string $collection, Response $response): self
    {
        $error = $response->json('error');
        $message = $response->json('message');

        return new self(
            collection: $collection,
            status: $response->status(),
            errorCode: is_string($error) ? $error : null,
            message: is_string($message) ? $message : null,
        );
    }

    /**
     * @return array<string, mixed>
     */
    public function context(): array
    {
        return [
            'collection' => $this->collection,
            'status' => $this->status,
            'error_code' => $this->errorCode,
        ];
    }
}
