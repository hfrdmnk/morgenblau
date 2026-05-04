<?php

namespace App\Services\Http;

interface DnsResolver
{
    /**
     * Resolve the host to a list of A/AAAA IPs (IPv4 + IPv6, in any order).
     * Empty list means "no records".
     *
     * @return list<string>
     */
    public function resolve(string $host): array;
}
