<?php

use Illuminate\Foundation\Inspiring;
use Illuminate\Support\Facades\Artisan;
use Illuminate\Support\Facades\Schedule;

Artisan::command('inspire', function () {
    $this->comment(Inspiring::quote());
})->purpose('Display an inspiring quote');

Schedule::command('feeds:refresh-all')->everyThirtyMinutes()->withoutOverlapping();
Schedule::command('feeds:retry-disabled')->dailyAt('03:00')->withoutOverlapping();
