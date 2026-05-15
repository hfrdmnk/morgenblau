import { useEffect } from 'react';

const APP_NAME = 'Morgenblau';

export function useDocumentTitle(title?: string | null) {
    useEffect(() => {
        document.title = title ? `${title} | ${APP_NAME}` : APP_NAME;
    }, [title]);
}
