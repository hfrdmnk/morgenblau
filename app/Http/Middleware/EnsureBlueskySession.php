<?php

namespace App\Http\Middleware;

use App\Models\User;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class EnsureBlueskySession
{
    /**
     * Detect a stale OAuth session at the start of a gated request so the user
     * gets bounced to /login on page load instead of mid-action. User::bluesky()
     * already short-circuits when the access token is fresh, so this is cheap
     * on the happy path; expired tokens trigger a lock-guarded refresh which
     * may throw AuthenticationException and surface to the global handler.
     *
     * @param  Closure(Request): Response  $next
     */
    public function handle(Request $request, Closure $next): Response
    {
        $user = $request->user();

        if ($user instanceof User && ! $request->routeIs('logout')) {
            $user->bluesky();
        }

        return $next($request);
    }
}
