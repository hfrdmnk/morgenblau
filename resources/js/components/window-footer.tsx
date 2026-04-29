import AppLogoIcon from '@/components/app-logo-icon';

export function WindowFooter() {
    return (
        <footer className="flex flex-col items-center gap-1.5 py-6">
            <div className="grid size-6 place-items-center rounded-full bg-foreground/[0.04] text-muted-foreground/70 shadow-[inset_0_1px_1.5px_rgba(0,0,0,0.06)] dark:bg-white/[0.04] dark:shadow-[inset_0_1px_1.5px_rgba(0,0,0,0.4)]">
                <AppLogoIcon className="size-3.5" />
            </div>
            <p className="font-handwritten text-2xs text-muted-foreground">
                Your calm window into the Atmosphere.
            </p>
        </footer>
    );
}
