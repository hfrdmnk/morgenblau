<?php

use SoloTerm\Solo\Commands\Command;
use SoloTerm\Solo\Commands\MakeCommand;
use SoloTerm\Solo\Commands\TestCommand;
use SoloTerm\Solo\Hotkeys;
use SoloTerm\Solo\Themes;

// Solo may not (should not!) exist in prod, so we have to
// check here first to see if it's installed.
if (! class_exists('\SoloTerm\Solo\Manager')) {
    return [
        //
    ];
}

return [
    /*
    |--------------------------------------------------------------------------
    | Themes
    |--------------------------------------------------------------------------
    */
    'theme' => env('SOLO_THEME', 'dark'),

    'themes' => [
        'light' => Themes\LightTheme::class,
        'dark' => Themes\DarkTheme::class,
    ],

    /*
    |--------------------------------------------------------------------------
    | Keybindings
    |--------------------------------------------------------------------------
    */
    'keybinding' => env('SOLO_KEYBINDING', 'default'),

    'keybindings' => [
        'default' => Hotkeys\DefaultHotkeys::class,
        'vim' => Hotkeys\VimHotkeys::class,
    ],

    /*
    |--------------------------------------------------------------------------
    | Commands
    |--------------------------------------------------------------------------
    |
    */
    'commands' => [
        // 'About' => 'php artisan solo:about',
        // Port 8000 is required: the revolution/laravel-bluesky package hardcodes
        // http://127.0.0.1:8000/ as the OAuth loopback redirect URI in dev.
        'Serve' => 'php artisan serve --port=8000',
        'Vite' => 'bun run dev',
        // For enhanced log viewing with vendor frame collapsing, see soloterm/vtail
        'Logs' => 'tail -f -n 100 '.storage_path('logs/laravel.log'),
        'Make' => new MakeCommand,
        'Queue' => Command::from('php artisan queue:work'),

        // Lazy commands do not automatically start when Solo starts.
        'Dumps' => Command::from('php artisan solo:dumps')->lazy(),
        'Pint' => Command::from('./vendor/bin/pint --ansi')->lazy(),
        'Tests' => TestCommand::artisan(),
    ],

    /*
    |--------------------------------------------------------------------------
    | Miscellaneous
    |--------------------------------------------------------------------------
    */

    /*
     * If you run the solo:dumps command, Solo will start a server to receive
     * the dumps. This is the address. You probably don't need to change
     * this unless the default is already taken for some reason.
     */
    'dump_server_host' => env('SOLO_DUMP_SERVER_HOST', 'tcp://127.0.0.1:9984'),
];
