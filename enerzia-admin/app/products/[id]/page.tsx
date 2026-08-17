'use client';

import { useEffect, useRef, useState } from 'react';
import { useRouter, useParams } from 'next/navigation';
import Link from 'next/link';
import { me } from '@/lib/api/auth';
import { getProduct, updateProduct, retireProduct, getUploadSignature } from '@/lib/api/products';
import { clearToken, getToken, onUnauthorized } from '@/lib/api/token';
import { isApiError } from '@/lib/api/errors';
import { PageShell } from '@/components/PageShell';
import type { AdminProduct, AdminImage } from '@/lib/api/types';

/* ----------------------------------------------------------------- helpers */

function parsePaise(rupeeStr: string): number {
  const n = parseFloat(rupeeStr);
  return isNaN(n) ? 0 : Math.round(n * 100);
}

function parseIntOr0(s: string): number {
  const n = parseInt(s, 10);
  return isNaN(n) ? 0 : n;
}

/* ---------------------------------------------------------------- form state */

interface Draft {
  name: string;
  family: string;
  form: string;
  stat: string;
  stat2: string;
  blurb: string;
  grad: string;
  positionStr: string;
  mrpStr: string;
  priceStr: string;
  stockStr: string;
  active: boolean;
  ratingScoreStr: string;
  ratingCountStr: string;
  badges: { title: string; subtitle: string }[];
  nutritionServingSize: string;
  nutritionRows: { key: string; value: string }[];
  images: AdminImage[];
}

function productToDraft(p: AdminProduct): Draft {
  return {
    name: p.name,
    family: p.family,
    form: p.form,
    stat: p.stat,
    stat2: p.stat2,
    blurb: p.blurb,
    grad: p.grad,
    positionStr: String(p.position),
    mrpStr: (p.mrp / 100).toFixed(2),
    priceStr: (p.price / 100).toFixed(2),
    stockStr: String(p.stock),
    active: p.active,
    ratingScoreStr: String(p.rating.score),
    ratingCountStr: String(p.rating.count),
    badges: p.badges.map((b) => ({ ...b })),
    nutritionServingSize: p.nutrition?.servingSize ?? '',
    nutritionRows: p.nutrition?.rows?.map((r) => ({ ...r })) ?? [],
    // alt is the one omitempty field on the wire, so it can arrive absent.
    // Normalise it here so the draft is always well-formed and every input
    // bound to it stays controlled.
    images: p.images.map((img) => ({ ...img, alt: img.alt ?? '' })),
  };
}

function buildPutBody(id: string, d: Draft): Record<string, unknown> {
  return {
    // id: omitted — server uses the path id; a mismatched body id is a 422 by design.
    // discountPercent and soldOut: intentionally omitted — computed outputs;
    // sending them causes a 422.
    id,                    // same as path — never changed; satisfies the "omit or match" rule
    name: d.name,
    family: d.family,
    form: d.form,
    stat: d.stat,
    stat2: d.stat2,
    blurb: d.blurb,
    grad: d.grad,
    position: parseIntOr0(d.positionStr),
    mrp: parsePaise(d.mrpStr),
    price: parsePaise(d.priceStr),
    stock: parseIntOr0(d.stockStr),
    active: d.active,
    rating: {
      score: parseFloat(d.ratingScoreStr) || 0,
      count: parseIntOr0(d.ratingCountStr),
    },
    badges: d.badges,
    nutrition:
      d.nutritionServingSize || d.nutritionRows.length > 0
        ? { servingSize: d.nutritionServingSize, rows: d.nutritionRows }
        : null,
    images: d.images,
  };
}

/* ------------------------------------------------------------------ page */

export default function EditorPage() {
  const router = useRouter();
  const params = useParams();
  const rawId = params['id'];
  const id = Array.isArray(rawId) ? (rawId[0] ?? '') : (rawId ?? '');

  const [ready, setReady] = useState(false);
  const [email, setEmail] = useState('');
  const [product, setProduct] = useState<AdminProduct | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [loadError, setLoadError] = useState('');

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [saveSuccess, setSaveSuccess] = useState(false);

  const [retiring, setRetiring] = useState(false);
  const [showRetireConfirm, setShowRetireConfirm] = useState(false);
  const [retireError, setRetireError] = useState('');

  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState('');

  const fileRef = useRef<HTMLInputElement>(null);

  /* auth guard */
  useEffect(() => {
    const tok = getToken();
    if (!tok) { router.replace('/login'); return; }
    let cancelled = false;
    me()
      .then((d) => { if (!cancelled) { setEmail(d.email); setReady(true); } })
      .catch(() => { clearToken(); if (!cancelled) router.replace('/login'); });
    return () => { cancelled = true; };
  }, [router]);

  useEffect(() => {
    onUnauthorized(() => { clearToken(); router.replace('/login'); });
  }, [router]);

  /* load product after auth */
  useEffect(() => {
    if (!ready || !id) return;
    let cancelled = false;
    getProduct(id)
      .then((p) => {
        if (!cancelled) {
          setProduct(p);
          setDraft(productToDraft(p));
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setLoadError(
            isApiError(err) && err.code === 'NOT_FOUND'
              ? 'Product not found.'
              : isApiError(err)
              ? err.message
              : 'Could not load product.',
          );
        }
      });
    return () => { cancelled = true; };
  }, [ready, id]);

  /* ----------------------------------------------------------- field helpers */

  function set<K extends keyof Draft>(field: K, value: Draft[K]) {
    setDraft((prev) => (prev ? { ...prev, [field]: value } : prev));
  }

  /* badges */
  function addBadge() {
    setDraft((p) => p ? { ...p, badges: [...p.badges, { title: '', subtitle: '' }] } : p);
  }
  function removeBadge(i: number) {
    setDraft((p) => p ? { ...p, badges: p.badges.filter((_, idx) => idx !== i) } : p);
  }
  function setBadgeField(i: number, field: 'title' | 'subtitle', value: string) {
    setDraft((p) =>
      p ? { ...p, badges: p.badges.map((b, idx) => (idx === i ? { ...b, [field]: value } : b)) } : p,
    );
  }

  /* nutrition rows */
  function addNutritionRow() {
    setDraft((p) => p ? { ...p, nutritionRows: [...p.nutritionRows, { key: '', value: '' }] } : p);
  }
  function removeNutritionRow(i: number) {
    setDraft((p) =>
      p ? { ...p, nutritionRows: p.nutritionRows.filter((_, idx) => idx !== i) } : p,
    );
  }
  function setNutritionRowField(i: number, field: 'key' | 'value', value: string) {
    setDraft((p) =>
      p
        ? {
            ...p,
            nutritionRows: p.nutritionRows.map((r, idx) =>
              idx === i ? { ...r, [field]: value } : r,
            ),
          }
        : p,
    );
  }

  /* images */
  function makePrimary(index: number) {
    setDraft((p) => {
      if (!p) return p;
      const imgs = [...p.images];
      const removed = imgs.splice(index, 1);
      const selected = removed[0];
      if (!selected) return p;
      return { ...p, images: [selected, ...imgs] };
    });
  }
  function removeImage(index: number) {
    setDraft((p) => p ? { ...p, images: p.images.filter((_, idx) => idx !== index) } : p);
  }
  function setImageAlt(index: number, alt: string) {
    setDraft((p) =>
      p
        ? { ...p, images: p.images.map((img, idx) => (idx === index ? { ...img, alt } : img)) }
        : p,
    );
  }

  async function handleImageUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = '';
    setUploadError('');
    setUploading(true);
    try {
      let sig;
      try {
        sig = await getUploadSignature();
      } catch (err) {
        if (isApiError(err) && err.code === 'INTERNAL') {
          throw new Error(
            'Cloudinary is not configured on this server. Add CLOUDINARY_* variables to enerzia-be/.env.',
          );
        }
        throw new Error(
          isApiError(err)
            ? `Could not get upload signature: ${err.message}`
            : 'Could not reach the server to prepare the upload.',
        );
      }

      const fd = new FormData();
      fd.append('file', file);
      fd.append('api_key', sig.apiKey);
      fd.append('timestamp', String(sig.timestamp));
      fd.append('folder', sig.folder);
      fd.append('signature', sig.signature);

      let res: Response;
      try {
        res = await fetch(`https://api.cloudinary.com/v1_1/${sig.cloudName}/image/upload`, {
          method: 'POST',
          body: fd,
        });
      } catch {
        throw new Error('Could not reach Cloudinary. Check your connection.');
      }

      if (!res.ok) {
        throw new Error(`Cloudinary rejected the upload (HTTP ${res.status}).`);
      }

      const data = (await res.json()) as { secure_url?: string; public_id?: string };
      const url = data.secure_url;
      const publicId = data.public_id;
      if (!url || !publicId) throw new Error('Cloudinary response was missing secure_url or public_id.');

      setDraft((p) => p ? { ...p, images: [...p.images, { url, publicId, alt: '' }] } : p);
    } catch (err) {
      setUploadError(err instanceof Error ? err.message : 'Upload failed.');
    } finally {
      setUploading(false);
    }
  }

  /* ---------------------------------------------------------- save */

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    if (!draft) return;
    setSaveError('');
    setSaveSuccess(false);
    setSaving(true);
    try {
      const updated = await updateProduct(id, buildPutBody(id, draft));
      // Re-initialize draft from the server response so computed values are fresh.
      setProduct(updated);
      setDraft(productToDraft(updated));
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    } catch (err) {
      if (isApiError(err) && err.fields.length > 0) {
        setSaveError(err.fields[0]?.message ?? err.message);
      } else {
        setSaveError(isApiError(err) ? err.message : 'Could not save. Check your connection.');
      }
    } finally {
      setSaving(false);
    }
  }

  /* ---------------------------------------------------------- retire */

  async function handleRetire() {
    setRetireError('');
    setRetiring(true);
    try {
      await retireProduct(id);
      router.push('/products');
    } catch (err) {
      setRetireError(isApiError(err) ? err.message : 'Could not retire product.');
    } finally {
      setRetiring(false);
    }
  }

  /* ---------------------------------------------------------- render */

  if (!ready) return <div className="loading-center">Loading…</div>;

  if (loadError) {
    return (
      <PageShell email={email}>
        <main className="page-content">
          <p className="error-msg">{loadError}</p>
          <Link href="/products" className="btn btn-ghost" style={{ marginTop: '1rem' }}>
            ← Back to products
          </Link>
        </main>
      </PageShell>
    );
  }

  if (!draft || !product) {
    return (
      <PageShell email={email}>
        <main className="page-content">
          <p className="muted">Loading product…</p>
        </main>
      </PageShell>
    );
  }

  return (
    <PageShell email={email}>
      <main className="page-content">
        <div className="list-toolbar">
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <Link href="/products" className="btn btn-ghost btn-sm">← Products</Link>
            <h2 className="page-title">{product.name}</h2>
            {!product.active && <span className="badge-retired">Retired</span>}
          </div>
        </div>

        <form onSubmit={handleSave} noValidate>
          {/* -------- Identity (read-only) -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Identity</h3>
            <div className="form-field">
              <label>Product ID</label>
              <input type="text" value={id} readOnly className="input-readonly" />
              <span className="form-hint">
                Permanent — appears in every URL, cart, and order. Cannot be changed.
              </span>
            </div>

            <div className="form-field">
              <label htmlFor="e-name">Name</label>
              <input id="e-name" type="text" value={draft.name}
                onChange={(e) => set('name', e.target.value)} />
            </div>

            <div className="form-row">
              <div className="form-field">
                <label htmlFor="e-family">Family</label>
                <input id="e-family" type="text" value={draft.family}
                  onChange={(e) => set('family', e.target.value)} />
              </div>
              <div className="form-field">
                <label htmlFor="e-form">Form</label>
                <select id="e-form" value={draft.form}
                  onChange={(e) => set('form', e.target.value)}>
                  <option value="">Select…</option>
                  <option value="Powder">Powder</option>
                  <option value="Tablets">Tablets</option>
                  <option value="Bundle">Bundle</option>
                </select>
              </div>
            </div>

            <div className="form-field">
              <label htmlFor="e-stat">Stat</label>
              <input id="e-stat" type="text" value={draft.stat}
                onChange={(e) => set('stat', e.target.value)} />
            </div>

            <div className="form-field">
              <label htmlFor="e-stat2">Stat 2</label>
              <input id="e-stat2" type="text" value={draft.stat2}
                onChange={(e) => set('stat2', e.target.value)} />
            </div>

            <div className="form-field">
              <label htmlFor="e-blurb">Blurb</label>
              <textarea id="e-blurb" rows={4} value={draft.blurb}
                onChange={(e) => set('blurb', e.target.value)} />
            </div>

            <div className="form-field">
              <label htmlFor="e-grad">Gradient</label>
              <input id="e-grad" type="text" value={draft.grad}
                onChange={(e) => set('grad', e.target.value)} />
            </div>
          </div>

          {/* -------- Pricing -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Pricing</h3>
            <p className="form-section-note">
              Amounts in rupees — stored as paise (× 100). Discount is computed from
              MRP and price; do not enter it.
            </p>
            <div className="form-row">
              <div className="form-field">
                <label htmlFor="e-mrp">MRP (₹)</label>
                <input id="e-mrp" type="number" min="0" step="0.01"
                  value={draft.mrpStr}
                  onChange={(e) => set('mrpStr', e.target.value)} />
              </div>
              <div className="form-field">
                <label htmlFor="e-price">Price (₹)</label>
                <input id="e-price" type="number" min="0" step="0.01"
                  value={draft.priceStr}
                  onChange={(e) => set('priceStr', e.target.value)} />
              </div>
            </div>
          </div>

          {/* -------- Inventory -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Inventory</h3>
            <div className="form-row">
              <div className="form-field">
                <label htmlFor="e-stock">Stock</label>
                <input id="e-stock" type="number" min="0" step="1"
                  value={draft.stockStr}
                  onChange={(e) => set('stockStr', e.target.value)} />
                <span className="form-hint form-hint-warning">
                  Written absolutely — the number you enter is exactly what is stored.
                  A reservation taken while this form is open will be overwritten.
                </span>
              </div>
              <div className="form-field">
                <label htmlFor="e-position">Position</label>
                <input id="e-position" type="number" min="0" step="1"
                  value={draft.positionStr}
                  onChange={(e) => set('positionStr', e.target.value)} />
              </div>
            </div>

            <div className="form-field">
              <label className="checkbox-label">
                <input type="checkbox" checked={draft.active}
                  onChange={(e) => set('active', e.target.checked)} />
                <span>Active</span>
              </label>
              <span className="form-hint">
                Unchecked means this product is hidden from the shop (retired).
                Check and save to restore it.
              </span>
            </div>
          </div>

          {/* -------- Rating -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Rating</h3>
            <div className="form-row">
              <div className="form-field">
                <label htmlFor="e-rscore">Score (0–5)</label>
                <input id="e-rscore" type="number" min="0" max="5" step="0.1"
                  value={draft.ratingScoreStr}
                  onChange={(e) => set('ratingScoreStr', e.target.value)} />
              </div>
              <div className="form-field">
                <label htmlFor="e-rcount">Review count</label>
                <input id="e-rcount" type="number" min="0" step="1"
                  value={draft.ratingCountStr}
                  onChange={(e) => set('ratingCountStr', e.target.value)} />
              </div>
            </div>
          </div>

          {/* -------- Images -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Images</h3>
            <p className="form-section-note">
              Index 0 is the primary shot the shop renders. Upload first, then Save —
              images are not attached to the product until saved.
            </p>

            {draft.images.length > 0 && (
              <div className="image-grid">
                {draft.images.map((img, i) => (
                  <div key={img.publicId || i} className={`image-item${i === 0 ? ' image-primary' : ''}`}>
                    {/* plain <img> — next/image needs remotePatterns config, overkill for admin */}
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img src={img.url} alt={img.alt || ''} className="image-thumb" />
                    {i === 0 && <span className="badge-primary">Primary</span>}
                    <div className="image-meta">
                      <input
                        type="text"
                        placeholder="Alt text"
                        // alt is omitempty on the wire: an image saved without
                        // one comes back with the key absent, and `undefined`
                        // flips this from a controlled input to an uncontrolled
                        // one. Never let it be undefined.
                        value={img.alt ?? ''}
                        onChange={(e) => setImageAlt(i, e.target.value)}
                        className="image-alt-input"
                      />
                    </div>
                    <div className="image-actions">
                      {i > 0 && (
                        <button type="button" className="btn btn-ghost btn-xs"
                          onClick={() => makePrimary(i)}>
                          Make primary
                        </button>
                      )}
                      <button type="button" className="btn btn-ghost btn-xs btn-danger-ghost"
                        onClick={() => removeImage(i)}>
                        Remove
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {draft.images.length < 5 ? (
              <div className="upload-area">
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/*"
                  style={{ display: 'none' }}
                  onChange={handleImageUpload}
                  disabled={uploading}
                />
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => fileRef.current?.click()}
                  disabled={uploading}
                >
                  {uploading ? 'Uploading…' : 'Add image'}
                </button>
                {uploading && <span className="upload-pending"> Uploading to Cloudinary…</span>}
              </div>
            ) : (
              <p className="form-hint">Maximum 5 images reached.</p>
            )}

            {uploadError && (
              <div className="error-msg" style={{ marginTop: '0.5rem' }}>{uploadError}</div>
            )}
          </div>

          {/* -------- Badges -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Badges</h3>
            {draft.badges.map((b, i) => (
              <div key={i} className="dynamic-row">
                <input type="text" placeholder="Title" value={b.title}
                  onChange={(e) => setBadgeField(i, 'title', e.target.value)} />
                <input type="text" placeholder="Subtitle" value={b.subtitle}
                  onChange={(e) => setBadgeField(i, 'subtitle', e.target.value)} />
                <button type="button" className="btn btn-ghost btn-xs btn-danger-ghost"
                  onClick={() => removeBadge(i)}>
                  ×
                </button>
              </div>
            ))}
            <button type="button" className="btn btn-ghost btn-sm" onClick={addBadge}>
              Add badge
            </button>
          </div>

          {/* -------- Nutrition -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Nutrition</h3>
            <div className="form-field">
              <label htmlFor="e-serving">Serving size</label>
              <input id="e-serving" type="text" value={draft.nutritionServingSize}
                placeholder="e.g. 5 g"
                onChange={(e) => set('nutritionServingSize', e.target.value)} />
            </div>
            {draft.nutritionRows.map((r, i) => (
              <div key={i} className="dynamic-row">
                <input type="text" placeholder="Nutrient" value={r.key}
                  onChange={(e) => setNutritionRowField(i, 'key', e.target.value)} />
                <input type="text" placeholder="Amount" value={r.value}
                  onChange={(e) => setNutritionRowField(i, 'value', e.target.value)} />
                <button type="button" className="btn btn-ghost btn-xs btn-danger-ghost"
                  onClick={() => removeNutritionRow(i)}>
                  ×
                </button>
              </div>
            ))}
            <button type="button" className="btn btn-ghost btn-sm" onClick={addNutritionRow}>
              Add row
            </button>
          </div>

          {/* -------- Save bar -------- */}
          {saveError && <div className="error-msg" role="alert">{saveError}</div>}
          {saveSuccess && <div className="success-msg">Saved.</div>}
          <div className="save-bar">
            <button type="submit" className="btn btn-primary" disabled={saving}>
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        </form>

        {/* -------- Retire section (outside form, separate action) -------- */}
        <div className="retire-section">
          <h3 className="form-section-title">Danger zone</h3>
          {!showRetireConfirm ? (
            <button
              type="button"
              className="btn btn-danger"
              onClick={() => setShowRetireConfirm(true)}
            >
              Retire product
            </button>
          ) : (
            <div className="retire-confirm">
              <p>
                This will hide <strong>{product.name}</strong> from the shop. Past orders still
                reference it. To restore it, uncheck Retired (set Active) and save.
              </p>
              {retireError && <div className="error-msg" style={{ marginTop: '0.5rem' }}>{retireError}</div>}
              <div className="retire-actions">
                <button
                  type="button"
                  className="btn btn-danger"
                  onClick={handleRetire}
                  disabled={retiring}
                >
                  {retiring ? 'Retiring…' : 'Confirm retire'}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => { setShowRetireConfirm(false); setRetireError(''); }}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </div>
      </main>
    </PageShell>
  );
}
