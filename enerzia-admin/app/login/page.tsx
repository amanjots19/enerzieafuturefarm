'use client';

import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { login, me } from '@/lib/api/auth';
import { clearToken, getToken, onUnauthorized, setToken } from '@/lib/api/token';
import { isApiError, isNetworkError } from '@/lib/api/errors';

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  // True while we are checking whether an existing token is still valid.
  const [checking, setChecking] = useState(true);
  const emailRef = useRef<HTMLInputElement>(null);

  // Session restore: if a token is already in sessionStorage, verify it with
  // /admin/me before rendering the form. A valid token skips sign-in entirely.
  useEffect(() => {
    const tok = getToken();
    if (!tok) {
      setChecking(false);
      return;
    }
    let cancelled = false;
    me()
      .then(() => {
        if (!cancelled) router.replace('/products');
      })
      .catch(() => {
        clearToken();
        if (!cancelled) setChecking(false);
      });
    return () => {
      cancelled = true;
    };
  }, [router]);

  // Register the global 401 handler. If any authenticated request later returns
  // 401, clear the token and come back here.
  useEffect(() => {
    onUnauthorized(() => {
      clearToken();
      router.replace('/login');
    });
  }, [router]);

  // Focus the email field once the form is visible.
  useEffect(() => {
    if (!checking) {
      emailRef.current?.focus();
    }
  }, [checking]);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      // Never log the password — not here, not in a catch block, not in dev.
      const data = await login(email, password);
      setToken(data.token);
      router.push('/products');
    } catch (err) {
      if (isApiError(err)) {
        // 422: show the first field-level message (ordered by the API).
        // 401, 429: show the server's message verbatim — it already has the
        //   right copy and a second vocabulary means two places to update.
        if (err.fields.length > 0) {
          setError(err.fields[0]?.message ?? err.message);
        } else {
          setError(err.message);
        }
      } else if (isNetworkError(err)) {
        setError('Could not reach the server. Check your connection.');
      } else {
        setError('Something went wrong. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  }

  if (checking) {
    return <div className="loading-center">Loading…</div>;
  }

  return (
    <div className="page-center">
      <div className="card">
        <h1 className="card-title">Sign in</h1>
        <form onSubmit={handleSubmit} noValidate>
          <div className="form-field">
            <label htmlFor="email">Email</label>
            <input
              ref={emailRef}
              id="email"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              disabled={loading}
              required
            />
          </div>
          <div className="form-field">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={loading}
              required
            />
          </div>
          <button type="submit" className="btn btn-primary" disabled={loading}>
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
          {error && <p className="error-msg" role="alert">{error}</p>}
        </form>
      </div>
    </div>
  );
}
