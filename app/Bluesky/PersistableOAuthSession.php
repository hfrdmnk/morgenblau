<?php

namespace App\Bluesky;

use Revolution\Bluesky\Session\OAuthSession;

/**
 * The package's parent::tokenExpired() only checks the JWT `exp` claim of the
 * access token. ATProto access tokens are implementation-defined and PDSes
 * like eurosky.social issue opaque tokens, so the JWT parse is empty and the
 * package treats them as expired on every request — triggering a refresh on
 * every authenticated page load. We trust the spec-mandated `expires_at`
 * (computed from `expires_in` at issuance) instead, with a small safety
 * buffer so we don't hand out a token that will expire mid-XRPC call.
 */
class PersistableOAuthSession extends OAuthSession
{
    private const SAFETY_BUFFER_SECONDS = 30;

    #[\Override]
    public function tokenExpired(): bool
    {
        $expiresAt = $this->get('expires_at');

        if (is_int($expiresAt) || (is_string($expiresAt) && ctype_digit($expiresAt))) {
            return now()->getTimestamp() >= ((int) $expiresAt) - self::SAFETY_BUFFER_SECONDS;
        }

        return parent::tokenExpired();
    }
}
