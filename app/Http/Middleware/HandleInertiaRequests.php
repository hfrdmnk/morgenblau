<?php

namespace App\Http\Middleware;

use App\Services\Profile\PdsProfileService;
use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Middleware;

class HandleInertiaRequests extends Middleware
{
    protected $rootView = 'app';

    /**
     * @return array<string, mixed>
     */
    public function share(Request $request): array
    {
        return [
            ...parent::share($request),
            'name' => config('app.name'),
            'auth' => [
                'user' => $request->user(),
                'handle' => $request->session()->get('atproto.handle'),
                'profile' => Inertia::defer(fn () => $request->user()
                    ? app(PdsProfileService::class)->for($request->user())
                    : null
                ),
            ],
            'flash' => fn () => $request->session()->get('flash'),
            'sidebarOpen' => ! $request->hasCookie('sidebar_state') || $request->cookie('sidebar_state') === 'true',
        ];
    }
}
