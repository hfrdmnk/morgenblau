<?php

namespace App\Exceptions;

use DomainException;

class AlreadySubscribedException extends DomainException
{
    public function __construct(public readonly string $feedUrl)
    {
        parent::__construct("Already subscribed to {$feedUrl}");
    }
}
