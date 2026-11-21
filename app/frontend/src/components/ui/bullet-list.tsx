import type { ComponentProps } from 'react';

// A list that keeps its bullets: Frame is a flex container, and a flex item can't draw a list marker.
function BulletList(props: Omit<ComponentProps<'ul'>, 'className' | 'style'>) {
    return <ul data-slot="bullet-list" className="list-disc space-y-2 pl-5" {...props} />;
}

function BulletItem(props: Omit<ComponentProps<'li'>, 'className' | 'style'>) {
    return <li data-slot="bullet-item" className="break-words" {...props} />;
}

export { BulletItem, BulletList };
