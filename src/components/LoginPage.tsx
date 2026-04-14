import { useState } from 'react';
import { useAuth } from '../lib/auth-context';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Window } from './Window';

export function LoginPage() {
	const [handle, setHandle] = useState('');
	const [error, setError] = useState('');
	const { signIn, status } = useAuth();

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault();
		const trimmed = handle.trim().replace(/^@/, '');
		if (!trimmed) return;

		setError('');
		try {
			await signIn(trimmed);
		} catch {
			setError('Could not sign in. Check your handle or DID and try again.');
		}
	};

	const isLoading = status === 'loading';

	return (
		<Window>
			<div className="flex h-full items-center justify-center px-6">
				<div className="w-full max-w-sm">
					<div className="mb-10 text-center">
						<p className="font-sans text-sm text-text-secondary">Your calm window into the</p>
						<h1 className="font-script text-6xl leading-none text-atmosphere-blue">Atmosphere</h1>
					</div>

					<form onSubmit={handleSubmit} className="space-y-3">
						<div className="space-y-1.5">
							<Label htmlFor="handle">Your Atmosphere account</Label>
							<Input
								id="handle"
								placeholder="your Atmosphere account"
								value={handle}
								onChange={(e) => setHandle(e.target.value)}
								disabled={isLoading}
								autoCapitalize="none"
								autoCorrect="off"
								spellCheck={false}
							/>
						</div>
						<Button type="submit" className="w-full" disabled={isLoading || !handle.trim()}>
							Login
						</Button>
						{error && <p className="text-sm text-destructive">{error}</p>}
					</form>

					<p className="mt-6 text-center text-xs leading-relaxed text-text-secondary">
						New here? An Atmosphere account is your identity (or user name) across Bluesky, Tangled,
						Leaflet, Grain and other AT Protocol apps.
					</p>
				</div>
			</div>
		</Window>
	);
}
