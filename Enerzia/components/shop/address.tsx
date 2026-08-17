import { addressDTOSummary, validateAddress } from '@/lib/shop/pricing';
import type { ShopActions } from '@/app/shop/useShop';
import type { ShopAction } from '@/lib/shop/reducer';
import type { Address, AddressDTO, ShopState } from '@/lib/shop/types';

import { ErrorText, Field } from './controls';

export function AddressForm({
  state,
  dispatch,
  actions,
  hasExisting,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
  hasExisting: boolean;
}) {
  const addrSaving = state.pending.addressSave;

  const addrField = (field: keyof Address, label: string, placeholder: string) => (
    <Field
      id={`f-addr-${field}`}
      label={label}
      value={state.addr[field]}
      placeholder={placeholder}
      inputMode={
        field === 'pin' || field === 'phone'
          ? 'numeric'
          : field === 'email'
          ? 'email'
          : 'text'
      }
      onChange={(value) => dispatch({ type: 'setAddr', field, value })}
      error={state.addrFieldErrors[field]}
    />
  );

  const handleSave = async () => {
    const clientErr = validateAddress(state.addr);
    if (clientErr) {
      dispatch({ type: 'setAddrFieldError', field: clientErr.field, message: clientErr.message });
      return;
    }
    await actions.saveAddress(state.addr);
  };

  return (
    <div className="addr-form">
      <div className="field-grid-2">
        {addrField('name', 'Full name', 'Ananya Sharma')}
        {addrField('email', 'Email for order updates', 'you@email.com')}
      </div>
      <div style={{ marginTop: 14 }}>
        {addrField('phone', 'Mobile number for delivery', '9876543210')}
      </div>
      <div style={{ marginTop: 14 }}>
        {addrField('line1', 'Flat, house no., building, street', '12, Anand Residency, MG Road')}
      </div>
      <div className="field-grid-3" style={{ marginTop: 14 }}>
        {addrField('city', 'City', 'Pune')}
        {addrField('state', 'State', 'Maharashtra')}
        {addrField('pin', 'PIN code', '411001')}
      </div>
      <div className="addr-form-actions">
        <button
          className="btn btn-primary"
          type="button"
          disabled={addrSaving}
          onClick={() => void handleSave()}
        >
          {addrSaving
            ? 'Saving…'
            : state.editingAddressId
            ? 'Update address'
            : 'Save address'}
        </button>
        {hasExisting && (
          <button
            className="btn btn-ghost"
            type="button"
            disabled={addrSaving}
            onClick={() => dispatch({ type: 'hideAddressForm' })}
          >
            Cancel
          </button>
        )}
      </div>
    </div>
  );
}

// Discriminated union: cart mode uses radio selection; manage mode uses set-as-default.
type AddressCardProps = {
  addr: AddressDTO;
  addrDeleting: boolean;
  onEdit: () => void;
  onDelete: () => void;
} & (
  | { mode: 'cart'; selected: boolean; onSelect: () => void }
  | { mode: 'manage'; onSetDefault: () => void }
);

export function AddressCard(props: AddressCardProps) {
  const { addr, addrDeleting, onEdit, onDelete } = props;

  if (props.mode === 'cart') {
    const { selected, onSelect } = props;
    return (
      <div className={`addr-opt${selected ? ' addr-opt--selected' : ''}`}>
        {/* label wraps the radio + visual dot + text — clicking anywhere here selects the address */}
        <label className="addr-opt-main">
          <input
            className="addr-radio"
            type="radio"
            name="delivery-address"
            checked={selected}
            onChange={onSelect}
          />
          <span className="addr-dot" aria-hidden="true" />
          <span className="addr-body">
            <span className="addr-label">{addr.name}</span>
            <span className="addr-sub">{addressDTOSummary(addr)}</span>
          </span>
        </label>
        {/* buttons are siblings of the label, never descendants of an interactive element */}
        <span className="addr-actions">
          <button className="btn btn-ghost addr-action-btn" type="button" onClick={onEdit}>
            Edit
          </button>
          <button
            className="btn btn-ghost addr-action-btn"
            type="button"
            disabled={addrDeleting}
            onClick={onDelete}
          >
            Delete
          </button>
        </span>
      </div>
    );
  }

  // manage mode — no radio, no selection state; set-as-default only for non-default addresses
  const { onSetDefault } = props;
  return (
    <div className="addr-opt">
      <div className="addr-opt-main addr-opt-main--static">
        <span className="addr-body">
          <span className="addr-label">
            {addr.name}
            {addr.isDefault && <span className="addr-default-tag">Default</span>}
          </span>
          <span className="addr-sub">{addressDTOSummary(addr)}</span>
        </span>
      </div>
      <span className="addr-actions">
        {!addr.isDefault && (
          <button
            className="btn btn-ghost addr-action-btn"
            type="button"
            onClick={onSetDefault}
          >
            Set as default
          </button>
        )}
        <button className="btn btn-ghost addr-action-btn" type="button" onClick={onEdit}>
          Edit
        </button>
        <button
          className="btn btn-ghost addr-action-btn"
          type="button"
          disabled={addrDeleting}
          onClick={onDelete}
        >
          Delete
        </button>
      </span>
    </div>
  );
}

// Re-export ErrorText so screens that use this module don't need a second controls import.
export { ErrorText };
