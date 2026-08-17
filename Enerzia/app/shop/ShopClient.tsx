'use client';

import { useEffect, useRef, useState } from 'react';
import Script from 'next/script';

import { AddressesScreen } from '@/components/shop/AddressesScreen';
import { CartScreen } from '@/components/shop/CartScreen';
import { DoneScreen } from '@/components/shop/DoneScreen';
import { LoginScreen } from '@/components/shop/LoginScreen';
import { OrdersScreen } from '@/components/shop/OrdersScreen';
import { PdpScreen } from '@/components/shop/PdpScreen';
import { ReviewScreen } from '@/components/shop/ReviewScreen';
import { ShopScreen } from '@/components/shop/ShopScreen';
import { TopBar } from '@/components/shop/TopBar';
import { Banner, SelectionPrompt } from '@/components/shop/controls';

import { useShop } from './useShop';

export function ShopClient() {
  const { state, dispatch, actions } = useShop();
  const lastScreen = useRef(state.screen);

  // Razorpay Checkout JS load state. The script is only rendered when the
  // shopper is on the review screen — no reason to load it on other routes.
  // ShopClient never unmounts across screen changes, so it stays loaded once
  // set and is ready for retries without reloading.
  const [razorpayReady, setRazorpayReady] = useState(false);

  // MSG91 widget script load state. Loaded only when the shopper is on the
  // login screen. ShopClient never unmounts, so it stays loaded on later visits.
  const [widgetReady, setWidgetReady] = useState(false);

  // Each screen is a full page swap — scroll to top the way a real navigation would.
  useEffect(() => {
    if (lastScreen.current !== state.screen) {
      lastScreen.current = state.screen;
      window.scrollTo(0, 0);
    }
  }, [state.screen]);

  // When the active PDP id changes, load its detail payload. loadProduct is
  // stable (empty useCallback deps), so this only re-fires on pdp id changes.
  useEffect(() => {
    if (state.pdp) void actions.loadProduct(state.pdp);
  }, [state.pdp, actions.loadProduct]);

  // Fetch the order list each time the shopper navigates to the orders screen.
  useEffect(() => {
    if (state.screen === 'orders') void actions.loadOrders();
  }, [state.screen, actions.loadOrders]);

  // Refresh the address list each time the shopper navigates to the addresses screen.
  useEffect(() => {
    if (state.screen === 'addresses') void actions.loadAddresses();
  }, [state.screen, actions.loadAddresses]);

  return (
    <div className="shop-shell">
      <TopBar
        count={
          state.user
            ? (state.cart?.itemCount ?? 0)
            : state.cartBuffer.reduce((s, i) => s + i.qty, 0)
        }
        booting={state.booting}
        user={state.user}
        signOut={actions.signOut}
        dispatch={dispatch}
      />
      <Banner
        message={state.banner}
        onDismiss={() => dispatch({ type: 'clearBanner' })}
        onRetry={state.products.length === 0 && !state.booting ? actions.retryBoot : undefined}
      />
      {!state.user && state.cartBuffer.length > 0 && (state.screen === 'shop' || state.screen === 'pdp') && (
        <SelectionPrompt
          count={state.cartBuffer.reduce((s, i) => s + i.qty, 0)}
          onGoToCart={() => dispatch({ type: 'requireAuth', dest: 'cart' })}
        />
      )}

      {/* Load the MSG91 widget only when the shopper is on the login screen.
          afterInteractive keeps it off every other route. ShopClient never
          unmounts, so widgetReady stays true if the shopper returns to login. */}
      {state.screen === 'login' && (
        <Script
          src="https://control.msg91.com/app/assets/otp-provider/otp-provider.js"
          strategy="afterInteractive"
          onReady={() => setWidgetReady(true)}
          onError={() => {
            dispatch({
              type: 'reqFail',
              key: 'widgetSession',
              message:
                'Sign-in widget failed to load. Please disable any ad blockers and refresh.',
            });
          }}
        />
      )}

      {/* Load Razorpay while the shopper reads the review screen. Strategy
          afterInteractive keeps it off pages that never take payment. */}
      {state.screen === 'review' && (
        <Script
          src="https://checkout.razorpay.com/v1/checkout.js"
          strategy="afterInteractive"
          onReady={() => setRazorpayReady(true)}
          onError={() => {
            dispatch({
              type: 'reqFail',
              key: 'checkout',
              message: 'Failed to load the payment provider. Please refresh and try again.',
            });
          }}
        />
      )}

      <main>
        {state.screen === 'shop' && (
          <ShopScreen state={state} dispatch={dispatch} actions={actions} />
        )}
        {state.screen === 'pdp' && (
          <PdpScreen state={state} dispatch={dispatch} actions={actions} />
        )}
        {state.screen === 'login' && (
          <LoginScreen state={state} dispatch={dispatch} actions={actions} widgetReady={widgetReady} />
        )}
        {state.screen === 'cart' && (
          <CartScreen state={state} dispatch={dispatch} actions={actions} />
        )}
        {state.screen === 'review' && (
          <ReviewScreen
            state={state}
            dispatch={dispatch}
            actions={actions}
            razorpayReady={razorpayReady}
          />
        )}
        {state.screen === 'done' && <DoneScreen state={state} dispatch={dispatch} />}
        {state.screen === 'orders' && (
          <OrdersScreen state={state} dispatch={dispatch} actions={actions} />
        )}
        {state.screen === 'addresses' && (
          <AddressesScreen state={state} dispatch={dispatch} actions={actions} />
        )}
      </main>
    </div>
  );
}
