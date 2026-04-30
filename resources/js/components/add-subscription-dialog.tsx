import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useForm } from '@inertiajs/react';
import type { FormEvent } from 'react';
import { toast } from 'sonner';

import { store } from '@/actions/App/Http/Controllers/SubscriptionController';
import InputError from '@/components/input-error';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

type FormShape = {
    url: string;
    is_private: boolean;
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
};

export function AddSubscriptionDialog({ open, onOpenChange }: Props) {
    const { data, setData, post, processing, errors, reset, clearErrors } =
        useForm<FormShape>({
            url: '',
            is_private: false,
        });

    const close = () => {
        reset();
        clearErrors();
        onOpenChange(false);
    };

    const handleOpenChange = (next: boolean) => {
        if (!next) {
            close();

            return;
        }

        onOpenChange(next);
    };

    const submit = (event: FormEvent) => {
        event.preventDefault();

        post(store().url, {
            preserveScroll: true,
            onSuccess: () => {
                toast.success('Source added.');
                close();
            },
        });
    };

    return (
        <Dialog open={open} onOpenChange={handleOpenChange}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Add a source</DialogTitle>
                    <DialogDescription>
                        Paste a website, RSS feed, YouTube channel, or podcast
                        URL.
                    </DialogDescription>
                </DialogHeader>

                <form onSubmit={submit} className="flex flex-col gap-5">
                    <div className="space-y-2">
                        <Label htmlFor="subscription-url" className="sr-only">
                            URL
                        </Label>
                        <Input
                            id="subscription-url"
                            type="url"
                            inputMode="url"
                            autoFocus
                            spellCheck={false}
                            placeholder="https://overreacted.io"
                            value={data.url}
                            onChange={(event) =>
                                setData('url', event.target.value)
                            }
                            aria-invalid={errors.url ? true : undefined}
                        />
                        <InputError message={errors.url} />
                    </div>

                    <label className="flex cursor-pointer items-start gap-3">
                        <Checkbox
                            checked={data.is_private}
                            onCheckedChange={(checked) =>
                                setData('is_private', checked === true)
                            }
                            className="mt-0.5"
                        />
                        <div className="flex flex-col gap-1 leading-snug">
                            <span className="text-sm font-medium">
                                Keep this private
                            </span>
                            <span className="font-handwritten text-base text-muted-foreground">
                                Stays in Morgenblau, not on your PDS.
                            </span>
                        </div>
                    </label>

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="ghost"
                            onClick={close}
                            disabled={processing}
                        >
                            Cancel
                        </Button>
                        <Button
                            type="submit"
                            disabled={processing || !data.url.trim()}
                        >
                            {processing ? (
                                <>
                                    <HugeiconsIcon
                                        icon={Loading03Icon}
                                        className="motion-safe:animate-spin"
                                    />
                                    Adding…
                                </>
                            ) : (
                                'Add source'
                            )}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
