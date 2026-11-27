import { useEffect, useState } from 'react';

import { pageCommands, type Command } from '@/lib/commands';

export function useCommands(commands: Command[]) {
    const [key] = useState(() => Symbol('commands'));
    useEffect(() => pageCommands.set(key, commands));
    useEffect(() => () => pageCommands.delete(key), [key]);
}
