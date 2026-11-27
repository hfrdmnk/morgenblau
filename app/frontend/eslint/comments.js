const POLICY =
    'Comments go stale and nothing enforces them. Make the code say it: a clearer name, an extracted function, a type or a union. If the next change must keep a rule, encode it as a test or lint rule whose failure message gives the reason. Put the reason for this change in the commit message. Only a note about something outside our control stays, and it links the upstream issue or docs. The baseline only covers comments that predate this check: never add to it.';

const link = /https?:\/\/[^\s"'<>()[\]`]+/g;
const ownLink =
    /^https?:\/\/(([a-z0-9-]+\.)*morgen\.blue([/:?#]|$)|(www\.)?(github\.com|tangled\.(org|sh))\/[^/]+\/morgenblau([/?#.]|$))/;
const license = /copyright|spdx-license-identifier|@license/i;
const marker = /^(\/\s*<reference\b|\s*prettier-ignore\b)/;
const eslintDisable = /^\s*eslint-disable/;

function groups(comments) {
    const groups = [];
    for (const comment of comments) {
        const last = groups.at(-1)?.at(-1);
        const continuesLineGroup =
            last?.type === 'Line' &&
            comment.type === 'Line' &&
            comment.loc.start.line === last.loc.start.line + 1 &&
            comment.loc.start.column === last.loc.start.column;
        if (continuesLineGroup) groups.at(-1).push(comment);
        else groups.push([comment]);
    }
    return groups;
}

const linksUpstream = (comment) => [...comment.value.matchAll(link)].some(([url]) => !ownLink.test(url));

const noComments = {
    meta: { type: 'suggestion', schema: [], messages: { policy: POLICY } },
    create(context) {
        const { sourceCode } = context;
        return {
            Program() {
                for (const group of groups(sourceCode.getAllComments())) {
                    const isLicenseHeader =
                        sourceCode.getTokenBefore(group[0]) === null && group.some(({ value }) => license.test(value));
                    if (isLicenseHeader || group.some(linksUpstream)) continue;
                    for (const comment of group) {
                        if (comment.type === 'Shebang' || marker.test(comment.value)) continue;
                        if (eslintDisable.test(comment.value)) continue;
                        context.report({ loc: comment.loc, messageId: 'policy' });
                    }
                }
            },
        };
    },
};

const noEslintDisable = {
    meta: {
        type: 'problem',
        schema: [],
        messages: {
            disable: `Fix the code instead of disabling the rule, or change the rule in eslint.config.js if it is wrong for the whole codebase. ${POLICY}`,
        },
    },
    create(context) {
        return {
            Program() {
                for (const comment of context.sourceCode.getAllComments()) {
                    if (eslintDisable.test(comment.value)) context.report({ loc: comment.loc, messageId: 'disable' });
                }
            },
        };
    },
};

export default {
    meta: { name: 'comments' },
    rules: { 'no-comments': noComments, 'no-eslint-disable': noEslintDisable },
};
