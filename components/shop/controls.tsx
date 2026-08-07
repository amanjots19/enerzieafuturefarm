import { offLabel, rupee } from '@/lib/shop/pricing';
import type { Variant } from '@/lib/shop/types';

/** Small pill used for filters and in-card variant selection. */
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

/** Two-line variant selector used on the product detail screen. */
export function PillBig({
  variant,
  active,
  onClick,
}: {
  variant: Variant;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button className="pill-big" type="button" aria-pressed={active} onClick={onClick}>
      <span className="pb-label">{variant.label}</span>
      <span className="pb-sub">{variant.sub}</span>
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
}) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <input
        className={`input${className ? ' ' + className : ''}`}
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
      />
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

export function PriceRow({ variant, className }: { variant: Variant; className: string }) {
  return (
    <div className={className}>
      <span className="price">{rupee(variant.price)}</span>
      <span className="mrp">{rupee(variant.mrp)}</span>
      <span className="off">{offLabel(variant)}</span>
    </div>
  );
}
