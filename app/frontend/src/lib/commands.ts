type Outcome = Promise<string | void> | string | void;

export type CommandOption = { value: string; label: string; detail?: string; shortcut?: string; step?: CommandStep };

export type CommandStep = (
    | {
          kind: 'text';
          title: string;
          initial: string;
          placeholder: string;
          action: (value: string) => string;
          problem?: (value: string) => string | null;
          submit: (value: string) => Outcome;
      }
    | {
          kind: 'choice';
          title: string;
          placeholder: string;
          options: CommandOption[];
          current?: string;
          submit: (value: string) => Outcome;
      }
    | {
          kind: 'tags';
          title: string;
          tags: string[];
          suggestionsPath: string;
          save: (tags: string[]) => Promise<unknown> | void;
      }
    | { kind: 'confirm'; title: string; description: string; action: string; submit: () => Outcome }
) & { parent?: CommandStep };

export type Command = {
    id: string;
    group: string;
    label: string;
    keywords?: string[];
    detail?: string;
    icon?: string | null;
    searchOnly?: boolean;
} & (
    { run: () => Outcome } | { toggle: { on: boolean; set: (on: boolean) => Promise<unknown> } } | { step: CommandStep }
);

export class CommandProblem extends Error {}

export async function copied(text: string, message: string) {
    await navigator.clipboard.writeText(text);
    return message;
}

export function openInTab(url: string) {
    window.open(url, '_blank', 'noopener,noreferrer');
}

export const GO_TO = 'Go to';
export const ACTIONS = 'Actions';

const registrations = new Map<symbol, Command[]>();
const listeners = new Set<() => void>();
let snapshot: Command[] = [];

function emit() {
    snapshot = [...registrations.values()].flat();
    listeners.forEach((listener) => listener());
}

export const pageCommands = {
    get: () => snapshot,
    subscribe(listener: () => void) {
        listeners.add(listener);
        return () => {
            listeners.delete(listener);
        };
    },
    set(key: symbol, commands: Command[]) {
        registrations.set(key, commands);
        emit();
    },
    delete(key: symbol) {
        registrations.delete(key);
        emit();
    },
};
