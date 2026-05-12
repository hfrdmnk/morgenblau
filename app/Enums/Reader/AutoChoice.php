<?php

namespace App\Enums\Reader;

use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
enum AutoChoice: string
{
    case Feed = 'feed';
    case Extracted = 'extracted';
}
