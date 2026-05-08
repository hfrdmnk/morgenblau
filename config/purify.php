<?php

use Stevebauman\Purify\Cache\CacheDefinitionCache;
use Stevebauman\Purify\Definitions\Html5Definition;

return [

    'default' => 'blogpost',

    'configs' => [

        'blogpost' => [
            'Core.Encoding' => 'utf-8',
            'HTML.Doctype' => 'HTML 4.01 Transitional',
            'HTML.Allowed' => 'p,a[href|title],br,strong,em,b,i,u,s,del,code,pre,blockquote,ul,ol,li,h2,h3,h4,h5,h6,img[src|alt|title|width|height],figure,figcaption,span',
            'URI.AllowedSchemes' => ['https' => true, 'http' => true, 'mailto' => true],
            'AutoFormat.AutoParagraph' => false,
            'AutoFormat.RemoveEmpty' => true,
        ],

        'microblog' => [
            'Core.Encoding' => 'utf-8',
            'HTML.Doctype' => 'HTML 4.01 Transitional',
            'HTML.Allowed' => 'p,a[href|title],br,strong,em',
            'URI.AllowedSchemes' => ['https' => true],
            'AutoFormat.AutoParagraph' => false,
            'AutoFormat.RemoveEmpty' => true,
        ],

        'video' => [
            'Core.Encoding' => 'utf-8',
            'HTML.Doctype' => 'HTML 4.01 Transitional',
            'HTML.Allowed' => 'p,a[href|title],br,strong,em,b,i,u,s,del,code,pre,blockquote,ul,ol,li,h2,h3,h4,h5,h6,img[src|alt|title|width|height],figure,figcaption,span',
            'URI.AllowedSchemes' => ['https' => true, 'http' => true, 'mailto' => true],
            'AutoFormat.AutoParagraph' => false,
            'AutoFormat.RemoveEmpty' => true,
        ],

        'podcast' => [
            'Core.Encoding' => 'utf-8',
            'HTML.Doctype' => 'HTML 4.01 Transitional',
            'HTML.Allowed' => 'p,a[href|title],br,strong,em,b,i,u,s,del,code,pre,blockquote,ul,ol,li,h2,h3,h4,h5,h6,img[src|alt|title|width|height],figure,figcaption,span',
            'URI.AllowedSchemes' => ['https' => true, 'http' => true, 'mailto' => true],
            'AutoFormat.AutoParagraph' => false,
            'AutoFormat.RemoveEmpty' => true,
        ],

    ],

    'definitions' => Html5Definition::class,

    'css-definitions' => null,

    'serializer' => [
        'driver' => env('CACHE_STORE', env('CACHE_DRIVER', 'file')),
        'cache' => CacheDefinitionCache::class,
    ],

];
