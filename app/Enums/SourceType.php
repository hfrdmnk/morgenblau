<?php

namespace App\Enums;

use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
enum SourceType: string
{
    case Rss = 'rss';
    case Video = 'video';
    case Podcast = 'podcast';
    case Microblog = 'microblog';
}
