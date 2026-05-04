<?php

namespace Tests\Doubles;

use App\Services\Http\DnsResolver;

class FakeDnsResolver implements DnsResolver
{
    /**
     * @param  array<string, list<string>>  $map  host => list of IPs
     */
    public function __construct(private readonly array $map) {}

    public function resolve(string $host): array
    {
        if (filter_var($host, FILTER_VALIDATE_IP)) {
            return [$host];
        }

        return $this->map[$host] ?? [];
    }
}
