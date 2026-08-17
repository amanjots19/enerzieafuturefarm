import { rupeeFromPaise } from '@/lib/shop/pricing';

/** Small pill used for filters. */
export function Pill({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button className="pill" type="button" aria-pressed={active} onClick={onClick}>
      {label}
    </button>
  );
}

/** Two-line pill used on the PDP for family-size navigation. */
export function PillBig({
  label,
  sub,
  active,
  soldOut,
  onClick,
}: {
  label: string;
  sub: string;
  active: boolean;
  soldOut: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={`pill-big${soldOut ? ' pill-big--sold-out' : ''}`}
      type="button"
      aria-pressed={active}
      disabled={soldOut}
      onClick={onClick}
    >
      <span className="pb-label">{label}</span>
      <span className="pb-sub">{sub}</span>
    </button>
  );
}

export function Field({
  id,
  label,
  value,
  placeholder,
  onChange,
  className,
  inputMode,
  autoComplete,
  onEnter,
  error,
}: {
  id: string;
  label: string;
  value: string;
  placeholder: string;
  onChange: (value: string) => void;
  className?: string;
  inputMode?: 'numeric' | 'text' | 'email' | 'tel';
  autoComplete?: string;
  onEnter?: () => void;
  error?: string;
}) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <input
        className={`input${className ? ' ' + className : ''}${error ? ' input--error' : ''}`}
        id={id}
        value={value}
        placeholder={placeholder}
        inputMode={inputMode}
        autoComplete={autoComplete}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={
          onEnter
            ? (e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  onEnter();
                }
              }
            : undefined
        }
        aria-invalid={error ? 'true' : undefined}
        aria-describedby={error ? `${id}-err` : undefined}
      />
      {error && (
        <div className="field-err" id={`${id}-err`} role="alert">
          {error}
        </div>
      )}
    </div>
  );
}

export function ErrorText({ children }: { children: string }) {
  return (
    <div className="err" role="alert">
      {children}
    </div>
  );
}

/**
 * Shown while the shopper has buffered items but is not yet signed in.
 * This is a positive affordance, not an error — uses its own green styling,
 * not the red .banner slot.
 */
export function SelectionPrompt({
  count,
  onGoToCart,
}: {
  count: number;
  onGoToCart: () => void;
}) {
  if (count === 0) return null;
  const label = count === 1 ? '1 item selected' : `${count} items selected`;
  return (
    <div className="selection-prompt" role="status">
      <span>{label} — Done selecting?</span>
      <button className="selection-prompt-cta" type="button" onClick={onGoToCart}>
        Go to cart →
      </button>
    </div>
  );
}

export function Banner({
  message,
  onDismiss,
  onRetry,
}: {
  message: string | null;
  onDismiss: () => void;
  onRetry?: () => void;
}) {
  if (message === null) return null;
  return (
    <div className="banner" role="alert">
      <span className="banner-msg">{message}</span>
      {onRetry && (
        <button className="banner-retry" type="button" onClick={onRetry}>
          Retry
        </button>
      )}
      <button className="banner-close" type="button" aria-label="Dismiss" onClick={onDismiss}>
        ×
      </button>
    </div>
  );
}

/** Paise price row with server-supplied discountPercent. */
export function PriceRow({
  price,
  mrp,
  discountPercent,
  className,
}: {
  price: number;
  mrp: number;
  discountPercent: number;
  className: string;
}) {
  return (
    <div className={className}>
      <span className="price">{rupeeFromPaise(price)}</span>
      <span className="mrp">{rupeeFromPaise(mrp)}</span>
      <span className="off">{discountPercent}% off</span>
    </div>
  );
}
