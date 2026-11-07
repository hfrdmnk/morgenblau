import { api } from '@/lib/api';

export async function downloadOpml() {
    const { opml } = await api<{ opml: string }>('/api/subscriptions/export', { method: 'POST' });
    const url = URL.createObjectURL(new Blob([opml], { type: 'text/x-opml;charset=utf-8' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = 'morgenblau.opml';
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    return 'Your OPML download is ready.';
}
