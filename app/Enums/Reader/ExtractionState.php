<?php

namespace App\Enums\Reader;

use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
enum ExtractionState: string
{
    case Available = 'available';
    case Pending = 'pending';
    case Failed = 'failed';
    case NotAttempted = 'not_attempted';
}
