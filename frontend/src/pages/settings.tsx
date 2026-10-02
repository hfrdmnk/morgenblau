import { NewsletterAddress } from '@/components/newsletter-address';
import { DataSettings } from '@/layouts/data-settings';

export function Settings() {
    return (
        <DataSettings page="general">
            <h1 className="text-2xl font-medium">General settings</h1>
            <div className="mt-12 max-w-xl">
                <NewsletterAddress settings />
            </div>
        </DataSettings>
    );
}
