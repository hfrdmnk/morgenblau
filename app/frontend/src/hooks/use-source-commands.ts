import { useLocation } from 'wouter';

import { useCommands } from '@/hooks/use-commands';
import type { FeedSourceDetail, NewsletterSourceDetail, SourceEntry, useSources } from '@/hooks/use-sources';
import { editSource, unsubscribe, youtubeFeeds } from '@/lib/add-source';
import { copied, openInTab, type Command } from '@/lib/commands';
import { entryHref, newsletterSourceHref, PATHS, sourceHref } from '@/lib/paths';
import { httpURL } from '@/lib/reader';
import { displayURL, feedTitle, newsletterTitle } from '@/lib/source-display';
import {
    deleteNewsletter,
    DELETION,
    editNewsletter,
    MAX_TITLE_LENGTH,
    newsletterEditable,
    REMOVAL,
    removeSource,
    resumeNewsletter,
    titleTooLong,
    unsubscribeNewsletter,
} from '@/lib/source-edit';

type Navigate = ReturnType<typeof useLocation>[1];
type Entries = ReturnType<typeof useSources<SourceEntry[]>>;

const TAG_SUGGESTIONS = '/api/subscriptions/tags';

function nameProblem(value: string): string | null {
    return titleTooLong(value) ? `Names can be up to ${MAX_TITLE_LENGTH} characters.` : null;
}

// The commands a feed's page offers; null while the page still loads, so its sample never reaches the menu.
export function useFeedSourceCommands(source: FeedSourceDetail | null, entries: Entries) {
    const [, navigate] = useLocation();
    const latest = entries.status === 'loaded' ? entries.data[0]?.entrySlug : undefined;
    useCommands(source ? [...feedEdits(source), ...feedLinks(source, latest, navigate), feedRemoval(source, navigate)] : []);
}

export function useNewsletterCommands(source: NewsletterSourceDetail | null, onChanged: () => void) {
    const [, navigate] = useLocation();
    useCommands(source ? [...newsletterEdits(source, onChanged), ...newsletterRemovals(source, onChanged, navigate), ...newsletterLinks(source, navigate)] : []);
}

function feedEdits(source: FeedSourceDetail): Command[] {
    const title = feedTitle(source);
    const tags = source.tags ?? [];
    const edit = (change: Parameters<typeof editSource>[1]) => editSource(source.rkey, change);
    return [
        {
            id: 'source:rename',
            group: title,
            label: 'Rename',
            keywords: ['title', 'name'],
            step: {
                kind: 'text',
                title: 'Rename',
                initial: title,
                placeholder: 'Use the feed’s own name',
                // An emptied name resets the source to its feed's own title, as the edit sheet does.
                action: (value) => (value.trim() ? `Rename to “${value.trim()}”` : 'Use the feed’s own name'),
                problem: nameProblem,
                submit: (value) => (value.trim() === title ? undefined : edit({ title: value.trim() }).then(() => 'Source renamed')),
            },
        },
        {
            id: 'source:tags',
            group: title,
            label: 'Edit tags',
            detail: tags.join(', '),
            step: { kind: 'tags', title: 'Tags', tags, suggestionsPath: TAG_SUGGESTIONS, save: (next) => edit({ tags: next }) },
        },
        { id: 'source:primary', group: title, label: 'Primary', keywords: ['first', 'top', 'pin'], toggle: { on: source.primary, set: (primary) => edit({ primary }) } },
        ...shortsToggle(source, title, edit),
    ];
}

function shortsToggle(source: FeedSourceDetail, title: string, edit: (change: { feedUrl: string }) => Promise<void>): Command[] {
    const feeds = source.kind === 'standardfeed' ? undefined : youtubeFeeds(source.feedUrl);
    if (!feeds) return [];
    return [{
        id: 'source:shorts',
        group: title,
        label: 'Include Shorts',
        keywords: ['youtube', 'video'],
        toggle: { on: source.feedUrl === feeds.all, set: (on) => edit({ feedUrl: on ? feeds.all : feeds.longForm }) },
    }];
}

function feedLinks(source: FeedSourceDetail, latest: string | undefined, navigate: Navigate): Command[] {
    const title = feedTitle(source);
    const site = httpURL(source.siteUrl);
    const commands: Command[] = [];
    if (site) commands.push({ id: 'source:site', group: title, label: 'Open site', detail: displayURL(site), run: () => openInTab(site) });
    if (source.kind !== 'standardfeed') commands.push({ id: 'source:copy-feed', group: title, label: 'Copy feed URL', run: () => copied(source.feedUrl, 'Feed URL copied') });
    if (latest) commands.push({ id: 'source:latest', group: title, label: 'Open latest post', run: () => navigate(entryHref(latest, sourceHref(source.rkey))) });
    return commands;
}

function feedRemoval(source: FeedSourceDetail, navigate: Navigate): Command {
    const title = feedTitle(source);
    return {
        id: 'source:delete',
        group: title,
        label: 'Delete source…',
        keywords: ['remove', 'unsubscribe', 'unfollow'],
        step: {
            kind: 'confirm',
            title: REMOVAL.feed.action,
            description: REMOVAL.feed.description,
            action: `Delete “${title}”`,
            submit: async () => {
                await unsubscribe(source.rkey);
                navigate(PATHS.sources);
                return REMOVAL.feed.done;
            },
        },
    };
}

function newsletterEdits(source: NewsletterSourceDetail, onChanged: () => void): Command[] {
    const title = newsletterTitle(source);
    const editable = newsletterEditable(source, source.status === 'stopped');
    const edit = (change: Parameters<typeof editNewsletter>[1]) => editNewsletter(editable, change).then(onChanged);
    return [
        {
            id: 'newsletter:rename',
            group: title,
            label: 'Rename',
            keywords: ['title', 'name'],
            step: {
                kind: 'text',
                title: 'Rename',
                initial: title,
                placeholder: 'A name for this newsletter',
                action: (value) => `Rename to “${value.trim()}”`,
                // A newsletter has no title of its own to fall back to.
                problem: (value) => (value.trim() ? nameProblem(value) : 'A newsletter needs a name.'),
                submit: (value) => (value.trim() === title ? undefined : edit({ title: value.trim() }).then(() => 'Newsletter renamed')),
            },
        },
        {
            id: 'newsletter:tags',
            group: title,
            label: 'Edit tags',
            detail: source.tags.join(', '),
            step: { kind: 'tags', title: 'Tags', tags: source.tags, suggestionsPath: TAG_SUGGESTIONS, save: (tags) => edit({ tags }) },
        },
        { id: 'newsletter:primary', group: title, label: 'Primary', keywords: ['first', 'top', 'pin'], toggle: { on: source.primary, set: (primary) => edit({ primary }) } },
    ];
}

function newsletterRemovals(source: NewsletterSourceDetail, onChanged: () => void, navigate: Navigate): Command[] {
    const title = newsletterTitle(source);
    const editable = newsletterEditable(source, source.status === 'stopped');
    const deleted = (done: string) => () => {
        navigate(PATHS.sources);
        return done;
    };
    const confirm = (id: string, label: string, copy: { action: string; description: string }, action: string, submit: () => Promise<string>): Command => ({
        id,
        group: title,
        label,
        keywords: ['remove'],
        step: { kind: 'confirm', title: copy.action, description: copy.description, action, submit },
    });
    const pause = editable.stopped
        ? { id: 'newsletter:resume', group: title, label: 'Resume newsletter', keywords: ['enable', 'start'], run: () => resumeNewsletter(editable).then(onChanged).then(() => 'Newsletter resumed') }
        : confirm('newsletter:stop', 'Stop newsletter…', REMOVAL.newsletter, `Stop “${title}”`, () => removeSource(editable).then(onChanged).then(() => REMOVAL.newsletter.done));
    const unsubscribing = source.canUnsubscribe
        ? [confirm('newsletter:unsubscribe', 'Unsubscribe and delete…', DELETION.automatic, `Unsubscribe from “${title}”`, () => unsubscribeNewsletter(editable).then(deleted(DELETION.automatic.done)))]
        : [];
    return [
        pause,
        ...unsubscribing,
        confirm('newsletter:delete', 'Delete newsletter…', DELETION.manual, `Delete “${title}”`, () => deleteNewsletter(editable).then(deleted(DELETION.manual.done))),
    ];
}

function newsletterLinks(source: NewsletterSourceDetail, navigate: Navigate): Command[] {
    const group = newsletterTitle(source);
    const latest = source.latestEntrySlug;
    return [
        { id: 'newsletter:copy-sender', group, label: 'Copy sender address', detail: source.senderAddress, run: () => copied(source.senderAddress, 'Sender address copied') },
        ...(latest ? [{ id: 'newsletter:latest', group, label: 'Open latest issue', run: () => navigate(entryHref(latest, newsletterSourceHref(source.id))) }] : []),
    ];
}
