import { createContext, useContext, useEffect, useState } from 'react';
import { initAuth, signIn as authSignIn, signOut as authSignOut, type Session } from './auth';

type AuthState =
	| { status: 'loading'; session: null }
	| { status: 'authenticated'; session: Session }
	| { status: 'unauthenticated'; session: null };

type AuthContextValue = AuthState & {
	signIn: (handle: string) => Promise<never>;
	signOut: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function useAuth(): AuthContextValue {
	const ctx = useContext(AuthContext);
	if (!ctx) throw new Error('useAuth must be used within AuthProvider');
	return ctx;
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
	const [state, setState] = useState<AuthState>({
		status: 'loading',
		session: null
	});

	useEffect(() => {
		initAuth()
			.then((session) => {
				setState(
					session
						? { status: 'authenticated', session }
						: { status: 'unauthenticated', session: null }
				);
			})
			.catch(() => {
				setState({ status: 'unauthenticated', session: null });
			});
	}, []);

	const handleSignOut = async () => {
		await authSignOut();
		setState({ status: 'unauthenticated', session: null });
	};

	return (
		<AuthContext value={{ ...state, signIn: authSignIn, signOut: handleSignOut }}>
			{children}
		</AuthContext>
	);
}
