'use client';

import { useEffect, useReducer, useRef } from 'react';

import { CartScreen } from '@/components/shop/CartScreen';
import { CheckoutScreen } from '@/components/shop/CheckoutScreen';
import { DoneScreen } from '@/components/shop/DoneScreen';
import { LoginScreen } from '@/components/shop/LoginScreen';
import { PdpScreen } from '@/components/shop/PdpScreen';
import { ShopScreen } from '@/components/shop/ShopScreen';
import { TopBar } from '@/components/shop/TopBar';
import { cartCount, initialState } from '@/lib/shop/pricing';
import { shopReducer } from '@/lib/shop/reducer';

export function ShopClient() {
  const [state, dispatch] = useReducer(shopReducer, initialState);
  const lastScreen = useRef(state.screen);

  /* Each screen is a full page swap, so start it at the top the way a real
     navigation would. */
  useEffect(() => {
    if (lastScreen.current !== state.screen) {
      lastScreen.current = state.screen;
      window.scrollTo(0, 0);
    }
  }, [state.screen]);

  return (
    <div className="shop-shell">
      <TopBar count={cartCount(state.cart)} dispatch={dispatch} />
      <main>
        {state.screen === 'shop' && <ShopScreen state={state} dispatch={dispatch} />}
        {state.screen === 'pdp' && <PdpScreen state={state} dispatch={dispatch} />}
        {state.screen === 'login' && <LoginScreen state={state} dispatch={dispatch} />}
        {state.screen === 'cart' && <CartScreen state={state} dispatch={dispatch} />}
        {state.screen === 'checkout' && <CheckoutScreen state={state} dispatch={dispatch} />}
        {state.screen === 'done' && <DoneScreen state={state} dispatch={dispatch} />}
      </main>
    </div>
  );
}
