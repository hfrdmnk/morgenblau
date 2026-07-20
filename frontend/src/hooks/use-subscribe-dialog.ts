import { useEffect, useState } from 'react';

import { api } from '@/lib/api';
import {
    classifySubscribeError,
    discoverSourceTitle,
    discoverSubscribeOverridePayload,
    pickSubscribeTopLevelError,
    type DiscoverSourceCard,
    type SubscribeErrorResult,
} from '@/lib/discover';
import {
    emitSubscriptionAdded,
    type AddedSubscription,
} from '@/lib/subscription-events';
import { mergeTagSuggestions } from '@/lib/tags';

type SubscribeResponseBody = {
    records?: AddedSubscription[];
    jobIds?: string[];
};

function parseSubscribeBody(body: SubscribeResponseBody | null | undefined) {
    if (!body) return { records: [], jobIds: [] };
    return { records: body.records ?? [], jobIds: body.jobIds ?? [] };
}

// Backs SubscribeDialog's form state, submit, and error surfaces for a single candidate card.
export function useSubscribeDialog(
    source: DiscoverSourceCard | null,
    open: boolean,
    onSuccess: () => void,
) {
    const [title, setTitle] = useState('');
    const [tags, setTags] = useState<string[]>([]);
    const [primary, setPrimary] = useState(false);
    const [seededKey, setSeededKey] = useState<string | null>(null);
    const [userTags, setUserTags] = useState<string[]>([]);
    const [submitting, setSubmitting] = useState(false);
    const [submitError, setSubmitError] = useState<string | undefined>(
        undefined,
    );
    const [fieldErrors, setFieldErrors] = useState<Record<string, string>>(
        {},
    );
    const [needsReauth, setNeedsReauth] = useState(false);

    const seed = (key: string | null, seedTitle: string) => {
        setSeededKey(key);
        setTitle(seedTitle);
        setTags([]);
        setPrimary(false);
        setSubmitError(undefined);
        setFieldErrors({});
        setNeedsReauth(false);
    };

    // Reseed when the dialog is pointed at a different card while still open (adjust-during-render, not an effect).
    if (source && source.key !== seededKey) {
        seed(source.key, discoverSourceTitle(source));
    }

    // Called from the Dialog's onOpenChangeComplete once the close animation finishes, so a reopened dialog starts fresh.
    const reset = () => seed(null, '');

    const onOpenChangeComplete = (next: boolean) => {
        if (!next) reset();
    };

    const titleError = fieldErrors['subscriptions.0.title'];
    const topLevelError = pickSubscribeTopLevelError(fieldErrors, submitError);

    // Pull the user's existing tags once per open, to seed suggestions.
    useEffect(() => {
        if (!open) return;
        let active = true;
        api<{ tags?: string[] }>('/api/subscriptions/tags')
            .then((body) => {
                if (active && body?.tags) setUserTags(body.tags);
            })
            .catch(() => {});
        return () => {
            active = false;
        };
    }, [open]);

    const applyError = (result: SubscribeErrorResult) => {
        if (result.kind === 'reauth') {
            setNeedsReauth(true);
            return;
        }
        if (result.kind === 'fields') {
            setFieldErrors(result.errors);
            return;
        }
        setSubmitError(result.message);
    };

    const submit = async () => {
        if (!source || submitting) return;
        setSubmitting(true);
        setSubmitError(undefined);
        setFieldErrors({});
        try {
            const body = await api<SubscribeResponseBody | undefined>(
                '/api/subscriptions',
                {
                    method: 'POST',
                    body: {
                        subscriptions: [
                            discoverSubscribeOverridePayload(source, {
                                title,
                                primary,
                                tags,
                            }),
                        ],
                    },
                },
            );
            emitSubscriptionAdded(parseSubscribeBody(body));
            onSuccess();
        } catch (err) {
            applyError(classifySubscribeError(err));
        } finally {
            setSubmitting(false);
        }
    };

    return {
        title,
        setTitle,
        tags,
        setTags,
        primary,
        setPrimary,
        tagSuggestions: mergeTagSuggestions(userTags, tags),
        submitting,
        titleError,
        titleInvalid: titleError ? true : undefined,
        topLevelError,
        needsReauth,
        submit,
        onOpenChangeComplete,
    };
}
