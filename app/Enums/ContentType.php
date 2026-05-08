<?php

namespace App\Enums;

use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
enum ContentType: string
{
    case Blogpost = 'blogpost';
    case Microblog = 'microblog';
    case Video = 'video';
    case Podcast = 'podcast';
}
