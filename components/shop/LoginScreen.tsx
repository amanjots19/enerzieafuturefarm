import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

import { ErrorText, Field } from './controls';

export function LoginScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  const otpStage = state.otpSent;

  return (
    <div className="wrap-sm screen">
      <div className="login-card">
        <h2>{otpStage ? 'Enter the code' : 'Sign in to continue'}</h2>
        <p className="login-sub">
          {otpStage
            ? `We sent a 6-digit code to +91 ${state.phone}`
            : 'We keep your cart, addresses and orders on your number.'}
        </p>

        {otpStage ? (
          <>
            <div style={{ marginBottom: 14 }}>
              <Field
                id="f-otp"
                label="6-digit code"
                value={state.otp}
                placeholder="••••••"
                className="otp-input"
                inputMode="numeric"
                autoComplete="one-time-code"
                onChange={(value) => dispatch({ type: 'setOtp', value })}
                onEnter={() => dispatch({ type: 'verifyOtp' })}
              />
            </div>
            {state.otpError && <ErrorText>Enter the 6-digit code (try 123456)</ErrorText>}
            <button
              className="btn btn-primary btn-block"
              type="button"
              style={{ padding: 13 }}
              onClick={() => dispatch({ type: 'verifyOtp' })}
            >
              Verify &amp; continue
            </button>
            <button
              className="btn btn-ghost btn-block"
              type="button"
              onClick={() => dispatch({ type: 'editPhone' })}
            >
              Change number
            </button>
          </>
        ) : (
          <>
            <div className="field" style={{ marginBottom: 14 }}>
              <label htmlFor="f-phone">Mobile number</label>
              <div className="phone-row">
                <span className="tag tag-neutral">+91</span>
                <input
                  className="input"
                  id="f-phone"
                  value={state.phone}
                  placeholder="98765 43210"
                  inputMode="numeric"
                  autoComplete="tel-national"
                  onChange={(e) => dispatch({ type: 'setPhone', value: e.target.value })}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      dispatch({ type: 'sendOtp' });
                    }
                  }}
                />
              </div>
            </div>
            {state.phoneError && <ErrorText>Enter a valid 10-digit mobile number</ErrorText>}
            <button
              className="btn btn-primary btn-block"
              type="button"
              style={{ padding: 13 }}
              onClick={() => dispatch({ type: 'sendOtp' })}
            >
              Send OTP
            </button>
            <p className="legal">
              By continuing you agree to Enerzeia Future Farm&rsquo;s terms and privacy policy.
            </p>
          </>
        )}
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
