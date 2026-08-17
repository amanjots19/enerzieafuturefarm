import type { ShopActions } from '@/app/shop/useShop';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

import { ErrorText } from './controls';
import { AddressCard, AddressForm } from './address';

export function AddressesScreen({
  state,
  dispatch,
  actions,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
}) {
  if (!state.user) {
    return (
      <div className="wrap-sm screen">
        <h1 className="page-title">Saved addresses</h1>
        <p className="muted">Sign in to manage your saved addresses.</p>
      </div>
    );
  }

  const hasAddresses = state.addresses.length > 0;
  const addrDeleting = state.pending.addressDelete;

  return (
    <div className="wrap-sm screen">
      <h1 className="page-title">Saved addresses</h1>

      {/* Address list — management mode, no radio selection */}
      {hasAddresses && !state.showAddressForm && (
        <>
          <div className="addr-list" role="list" aria-label="Saved addresses">
            {state.addresses.map((addr) => (
              <AddressCard
                key={addr.id}
                mode="manage"
                addr={addr}
                addrDeleting={addrDeleting}
                onSetDefault={() => void actions.setDefaultAddress(addr.id)}
                onEdit={() => dispatch({ type: 'showAddressForm', editing: addr.id })}
                onDelete={() => void actions.deleteAddress(addr.id)}
              />
            ))}
          </div>
          <button
            className="btn btn-ghost addr-add-btn"
            type="button"
            onClick={() => dispatch({ type: 'showAddressForm', editing: null })}
          >
            + Add new address
          </button>
          {state.addrError && (
            <div style={{ marginTop: 8 }}>
              <ErrorText>{state.addrError}</ErrorText>
            </div>
          )}
        </>
      )}

      {/* Empty state — invitation, not an error */}
      {!hasAddresses && !state.showAddressForm && (
        <div className="addr-empty">
          <p>No saved addresses yet.</p>
          <button
            className="btn btn-primary"
            type="button"
            onClick={() => dispatch({ type: 'showAddressForm', editing: null })}
          >
            Add your first address
          </button>
        </div>
      )}

      {/* Add / edit form */}
      {state.showAddressForm && (
        <AddressForm
          state={state}
          dispatch={dispatch}
          actions={actions}
          hasExisting={hasAddresses}
        />
      )}
    </div>
  );
}
