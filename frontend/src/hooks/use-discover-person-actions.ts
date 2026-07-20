import type { Dispatch, SetStateAction } from 'react';

import { api } from '@/lib/api';
import {
    personHidePayload,
    type PersonCard,
    type PersonSuggestionState,
} from '@/lib/discover-people';
import { parseFollowResponse, type FollowRecord } from '@/lib/follow';

type FollowBody = { rkey?: string; subjectDid?: string; createdAt?: string };

type Options = {
    followingDid: string | undefined;
    setFollowingDid: Dispatch<SetStateAction<string | undefined>>;
    onFollowed: (did: string, record: FollowRecord) => void;
};

// Suggestion-card follow/hide against the discover-people endpoints. Following marks the card inert
// in place (sources' subscribe grammar) and hands the record up to seed the follow list; it never
// removes the card. Hide removes optimistically and rolls back on failure. followingDid gates a
// single in-flight follow, lifted to the caller so the badge and disable state read one source.
export function useDiscoverPersonActions(
    state: PersonSuggestionState,
    setState: Dispatch<SetStateAction<PersonSuggestionState>>,
    options: Options,
) {
    const onFollow = (person: PersonCard, handle: string | undefined) => {
        if (!handle || options.followingDid) return;
        options.setFollowingDid(person.did);
        void (async () => {
            try {
                const body = await api<FollowBody>('/api/follows', {
                    method: 'POST',
                    body: { handle },
                });
                const record = parseFollowResponse(body);
                if (record) options.onFollowed(person.did, record);
            } catch {
                // Best-effort: leave the card followable so the user can retry.
            } finally {
                options.setFollowingDid(undefined);
            }
        })();
    };

    const onHide = async (person: PersonCard) => {
        if (state.kind !== 'ok') return;
        const index = state.people.findIndex((p) => p.did === person.did);
        if (index === -1) return;

        setState((prev) =>
            prev.kind === 'ok'
                ? {
                      ...prev,
                      people: prev.people.filter((p) => p.did !== person.did),
                  }
                : prev,
        );

        try {
            await api('/api/discover/hides', {
                method: 'POST',
                body: personHidePayload(person.did),
            });
        } catch {
            setState((prev) => {
                if (prev.kind !== 'ok') return prev;
                const people = prev.people.slice();
                people.splice(index, 0, person);
                return { ...prev, people };
            });
        }
    };

    return { onFollow, onHide };
}
