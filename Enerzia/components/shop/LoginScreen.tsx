import type { ShopActions } from '@/app/shop/useShop';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

/**
 * Tries to pull the access token out of whatever the MSG91 widget passes to
 * the success callback. The payload shape is undocumented; a real sign-in
 * already proved that `data['access-token']` is not always right (a 422 with
 * an empty body was the evidence). We try every known variant in order and
 * return null if none yields a non-empty string.
 *
 * @param payload - raw value from the widget success callback (unknown)
 * @returns the token string, or null if nothing found
 */
function extractToken(payload: unknown): string | null {
  // Case 1: the argument itself is the token string (some SDK versions do this)
  if (typeof payload === 'string' && payload.length > 0) return payload;

  if (payload !== null && typeof payload === 'object') {
    const obj = payload as Record<string, unknown>;
    // Cases 2-4: documented and observed key variants
    for (const key of ['access-token', 'message', 'accessToken']) {
      const val = obj[key];
      if (typeof val === 'string' && val.length > 0) return val;
    }
  }

  return null;
}

/**
 * Logs the SHAPE of the payload (keys + typeof each value) without logging
 * the values themselves — one of them is a credential.
 */
function warnPayloadShape(payload: unknown): void {
  if (payload !== null && typeof payload === 'object') {
    const shape = Object.fromEntries(
      Object.keys(payload as Record<string, unknown>).map((k) => [
        k,
        typeof (payload as Record<string, unknown>)[k],
      ]),
    );
    console.warn('[msg91] unexpected widget payload shape:', shape);
  } else {
    console.warn('[msg91] unexpected widget payload type:', typeof payload);
  }
}

export function LoginScreen({
  state,
  dispatch,
  actions,
  widgetReady,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
  widgetReady: boolean;
}) {
  const busy = state.pending.widgetSession;

  const launchWidget = () => {
    // Re-check at click time — an ad blocker can silence onReady and onError both.
    if (typeof window.initSendOTP !== 'function') {
      dispatch({
        type: 'reqFail',
        key: 'widgetSession',
        message:
          'Sign-in widget failed to load. Please disable any ad blockers and refresh.',
      });
      return;
    }
    window.initSendOTP({
      widgetId: process.env.NEXT_PUBLIC_MSG91_WIDGET_ID ?? '',
      tokenAuth: process.env.NEXT_PUBLIC_MSG91_TOKEN_AUTH ?? '',
      success: (data) => {
        const token = extractToken(data);
        if (token === null) {
          // Log shape for diagnostics — never the values (one is a credential).
          warnPayloadShape(data);
          dispatch({
            type: 'reqFail',
            key: 'widgetSession',
            message: 'Sign-in did not return a token. Please try again.',
          });
          return;
        }
        void actions.signInWithWidget(token);
      },
      failure: () => {
        dispatch({
          type: 'reqFail',
          key: 'widgetSession',
          message: 'Sign-in was not completed. Please try again.',
        });
      },
    });
  };

  return (
    <div className="wrap-sm screen">
      <div className="login-card">
        <h2>Sign in to continue</h2>
        <p className="login-sub">
          We keep your cart, addresses and orders on your number.
        </p>

        {/* MSG91 widget renders here when initSendOTP is called */}
        <div id="sendOtp" />

        <button
          className="btn btn-primary btn-block"
          type="button"
          style={{ padding: 13 }}
          disabled={!widgetReady || busy}
          onClick={launchWidget}
        >
          {busy ? 'Signing in…' : widgetReady ? 'Sign in with OTP' : 'Loading…'}
        </button>

        <p className="legal">
          By continuing you agree to Enerzeia Future Farm&rsquo;s terms and privacy policy.
        </p>
      </div>

      <button
        className="btn btn-ghost btn-block"
        type="button"
        style={{ marginTop: 10 }}
        onClick={() => dispatch({ type: 'go', screen: 'shop' })}
      >
        ← Keep shopping
      </button>
    </div>
  );
}
