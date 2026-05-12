<?php

namespace App\Enums\Reader;

use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
enum ExtractionFailureReason: string
{
    case Unreachable = 'unreachable';
    case NoContent = 'no_content';
}
